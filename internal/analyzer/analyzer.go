package analyzer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"project-helper/internal/ai"
	"project-helper/internal/repo"
	"project-helper/internal/store"
)

type Analyzer struct {
	store      *store.Store
	repos      *repo.Service
	ai         ai.Client
	reportsDir string
	hub        *hub
	mu         sync.Mutex
	running    map[int64]struct{}
}

func New(store *store.Store, repos *repo.Service, aiClient ai.Client, reportsDir string) *Analyzer {
	return &Analyzer{
		store:      store,
		repos:      repos,
		ai:         aiClient,
		reportsDir: reportsDir,
		hub:        newHub(),
		running:    make(map[int64]struct{}),
	}
}

func (a *Analyzer) Subscribe(projectID int64) (<-chan Event, func()) {
	ch := a.hub.subscribe(projectID)
	return ch, func() { a.hub.unsubscribe(projectID, ch) }
}

func (a *Analyzer) Start(ctx context.Context, project *store.Project, parsed repo.ParsedURL) (*store.Run, bool, error) {
	return a.start(ctx, project, parsed, false)
}

func (a *Analyzer) Regenerate(ctx context.Context, project *store.Project, parsed repo.ParsedURL) (*store.Run, bool, error) {
	return a.start(ctx, project, parsed, true)
}

func (a *Analyzer) start(ctx context.Context, project *store.Project, parsed repo.ParsedURL, force bool) (*store.Run, bool, error) {
	a.mu.Lock()
	if _, ok := a.running[project.ID]; ok {
		a.mu.Unlock()
		run, _ := a.store.LatestRun(ctx, project.ID)
		return run, false, nil
	}
	a.running[project.ID] = struct{}{}
	a.mu.Unlock()

	run, err := a.store.CreateRun(ctx, project.ID)
	if err != nil {
		a.markDone(project.ID)
		return nil, false, err
	}
	go a.run(context.Background(), run.ID, project.ID, parsed, force)
	return run, true, nil
}

func (a *Analyzer) run(ctx context.Context, runID, projectID int64, parsed repo.ParsedURL, force bool) {
	defer a.markDone(projectID)
	fail := func(step string, err error) {
		message := err.Error()
		_ = a.store.FinishRun(ctx, runID, store.StatusFailed, step, message, message)
		_ = a.store.SetProjectStatus(ctx, projectID, store.StatusFailed)
		a.hub.publish(projectID, Event{Type: "error", Step: step, Progress: 100, Error: message})
	}
	progress := func(step string, pct int, message string) {
		_ = a.store.UpdateRun(ctx, runID, store.StatusRunning, step, pct, message)
		_ = a.store.SetProjectStatus(ctx, projectID, store.StatusRunning)
		a.hub.publish(projectID, Event{Type: step, Step: step, Progress: pct, Message: message})
	}

	progress("queued", 5, "准备读取 GitHub 仓库信息")
	rev, err := a.repos.ResolveRevision(ctx, parsed)
	if err != nil {
		fail("queued", err)
		return
	}
	if err := a.store.UpdateProjectRevision(ctx, projectID, rev.DefaultBranch, rev.CommitSHA, store.StatusRunning); err != nil {
		fail("queued", err)
		return
	}

	hasReport, err := a.store.HasReport(ctx, projectID, rev.CommitSHA)
	if !force && err == nil && hasReport {
		_ = a.store.FinishRun(ctx, runID, store.StatusCompleted, "done", "命中缓存，已复用历史分析报告", "")
		_ = a.store.SetProjectStatus(ctx, projectID, store.StatusCompleted)
		a.hub.publish(projectID, Event{Type: "done", Step: "done", Progress: 100, Message: "命中缓存，已复用历史分析报告"})
		return
	}

	progress("cloning", 18, "正在克隆仓库")
	root, err := a.repos.Clone(ctx, parsed, rev)
	if err != nil {
		fail("cloning", err)
		return
	}

	progress("indexing", 38, "正在扫描目录和源码文件")
	files, tree, err := repo.IndexFiles(ctx, root, projectID, rev.CommitSHA)
	if err != nil {
		fail("indexing", err)
		return
	}
	if err := a.store.ReplaceFileIndex(ctx, projectID, rev.CommitSHA, files); err != nil {
		fail("indexing", err)
		return
	}

	progress("summarizing", 66, "正在整理技术栈和核心模块")
	techStack := detectTechStack(files)
	contextText := buildAnalysisContext(parsed, rev, root, files, tree, techStack)

	progress("reporting", 82, "正在调用 DeepSeek 生成通俗分析报告")
	reportStream, reportErrs := a.ai.Stream(ctx, []ai.Message{
		{Role: "system", Content: reportSystemPrompt()},
		{Role: "user", Content: contextText},
	})
	var reportBuilder strings.Builder
	for event := range reportStream {
		if event.Type != "token" || event.Data == "" {
			continue
		}
		reportBuilder.WriteString(event.Data)
		a.hub.publish(projectID, Event{Type: "report_token", Step: "reporting", Progress: 82, Data: event.Data})
	}
	for err := range reportErrs {
		if err != nil {
			fail("reporting", err)
			return
		}
	}
	report := reportBuilder.String()
	if strings.TrimSpace(report) == "" {
		fail("reporting", fmt.Errorf("模型返回了空报告"))
		return
	}
	techJSON, _ := json.Marshal(techStack)
	treeJSON, _ := json.Marshal(tree)
	if err := a.store.SaveReport(ctx, projectID, rev.CommitSHA, report, string(techJSON), string(treeJSON)); err != nil {
		fail("reporting", err)
		return
	}
	reportPath, err := a.writeReportFile(parsed, rev, report)
	if err != nil {
		fail("reporting", err)
		return
	}
	_ = a.store.FinishRun(ctx, runID, store.StatusCompleted, "done", "分析完成", "")
	_ = a.store.SetProjectStatus(ctx, projectID, store.StatusCompleted)
	a.hub.publish(projectID, Event{Type: "done", Step: "done", Progress: 100, Message: "分析完成，报告已导出到 " + reportPath})
}

func (a *Analyzer) markDone(projectID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.running, projectID)
}

func reportSystemPrompt() string {
	return `你是 project-helper，一个面向源码初学者的项目学习助手。

你的任务是：基于已有代码信息，生成一份完整、清晰、结构化的 Markdown 源码分析报告，帮助读者快速理解整个项目，而不是逐行解释代码。

请始终使用中文输出。

【总体要求】

1. 讲解必须通俗易懂，让没有经验的初学者也能理解，但不能简化到失去技术价值
2. 不要堆砌代码或复述文件内容，要做“总结 + 解释 + 抽象”
3. 优先解释“为什么这样设计”，而不是“代码写了什么”
4. 所有分析必须基于已有文件信息，严禁编造不存在的内容
5. 遇到不确定或不完整的信息，必须明确说明：“根据文件推断”

【输出格式（必须严格遵守）】

使用 Markdown，包含以下结构：

# 项目概述

* 项目是做什么的（核心目标）
* 解决什么问题
* 一句话总结

# 技术栈

* 使用了哪些语言 / 框架 / 工具
* 每个技术的作用（简单解释）

# 目录结构

用树状或分层方式说明关键目录，并解释作用
示例：

* cmd/：程序入口
* internal/server/：服务核心逻辑

# 核心模块分析

按模块拆解，而不是按文件罗列

每个模块包含：

* 模块作用（它解决什么问题）
* 关键文件（用反引号标出路径）
* 模块之间的关系

# 数据流 / 调用流程（重点）

说明系统是如何运行的：

* 请求是如何进入系统的
* 中间经过哪些模块
* 最终如何输出结果

尽量用流程化方式表达，并辅以类比帮助理解

# 设计思路与模式

分析项目中的设计特点，例如：

* 分层架构（如 controller / service / repo）
* 常见设计模式（工厂、依赖注入等）
* 为什么这样设计

# 阅读建议

给初学者一个清晰路径，例如：

1. 先看 cmd/main.go 理解入口
2. 再看 internal/server/ 理解整体流程
3. 最后深入具体业务模块

【表达要求】

* 多用类比解释抽象概念（例如“餐厅点单流程”“流水线”）
* 避免使用晦涩术语，如必须使用要解释
* 使用分点、分段，避免大段文字
* 控制篇幅：完整但不过度冗长

【默认策略】

如果信息有限：

* 优先构建整体结构理解
* 对不确定部分标注“根据文件推断

`

}

func buildAnalysisContext(parsed repo.ParsedURL, rev repo.Revision, root string, files []store.FileRecord, tree repo.Tree, techStack []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "仓库：%s\n默认分支：%s\nCommit：%s\n本地路径名：%s\n\n", parsed.Normalized, rev.DefaultBranch, rev.CommitSHA, filepath.Base(root))
	fmt.Fprintf(&b, "识别出的技术栈：%s\n\n", strings.Join(techStack, ", "))
	b.WriteString("目录树摘要：\n")
	writeTree(&b, tree, 0, 3)
	b.WriteString("\n关键文件摘要：\n")
	for _, file := range pickImportantFiles(files, 80) {
		fmt.Fprintf(&b, "- `%s` [%s, %d bytes]: %s\n", file.Path, file.Language, file.Size, file.Summary)
	}
	return b.String()
}

func writeTree(b *strings.Builder, tree repo.Tree, depth, maxDepth int) {
	if depth > maxDepth {
		return
	}
	if tree.Path != "." {
		fmt.Fprintf(b, "%s- %s/\n", strings.Repeat("  ", depth), tree.Name)
	}
	for _, child := range tree.Children {
		if child.Type == "file" {
			fmt.Fprintf(b, "%s- %s\n", strings.Repeat("  ", depth+1), child.Name)
			continue
		}
		writeTree(b, child, depth+1, maxDepth)
	}
}

func detectTechStack(files []store.FileRecord) []string {
	found := map[string]bool{}
	for _, file := range files {
		name := strings.ToLower(file.Path)
		switch {
		case strings.HasSuffix(name, "go.mod"):
			found["Go"] = true
		case strings.HasSuffix(name, "package.json"):
			found["Node.js"] = true
			if strings.Contains(file.Content, `"vue"`) {
				found["Vue"] = true
			}
			if strings.Contains(file.Content, `"react"`) {
				found["React"] = true
			}
		case strings.HasSuffix(name, "vite.config.js") || strings.HasSuffix(name, "vite.config.ts"):
			found["Vite"] = true
		case strings.HasSuffix(name, "requirements.txt") || strings.HasSuffix(name, "pyproject.toml"):
			found["Python"] = true
		case strings.Contains(name, "dockerfile") || strings.HasSuffix(name, "docker-compose.yml"):
			found["Docker"] = true
		}
		if file.Language != "Text" && file.Language != "JSON" && file.Language != "YAML" && file.Language != "Markdown" {
			found[file.Language] = true
		}
	}
	var tech []string
	for item := range found {
		tech = append(tech, item)
	}
	sort.Strings(tech)
	if len(tech) == 0 {
		return []string{"根据文件结构推断为通用源码项目"}
	}
	return tech
}

func pickImportantFiles(files []store.FileRecord, limit int) []store.FileRecord {
	score := func(file store.FileRecord) int {
		name := strings.ToLower(file.Path)
		points := 0
		if strings.Contains(name, "readme") || strings.HasSuffix(name, "go.mod") || strings.HasSuffix(name, "package.json") {
			points += 100
		}
		if strings.Contains(name, "main.") || strings.Contains(name, "app.") || strings.Contains(name, "server.") || strings.Contains(name, "router") {
			points += 50
		}
		if strings.Contains(name, "cmd/") || strings.Contains(name, "internal/") || strings.Contains(name, "src/") {
			points += 20
		}
		points -= strings.Count(file.Path, "/") * 2
		return points
	}
	out := append([]store.FileRecord(nil), files...)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i]), score(out[j])
		if si == sj {
			return out[i].Path < out[j].Path
		}
		return si > sj
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func EnsureReportReady(ctx context.Context, st *store.Store, project *store.Project) error {
	if project.CommitSHA == "" {
		return sql.ErrNoRows
	}
	_, err := st.GetReport(ctx, project.ID, project.CommitSHA)
	return err
}

var reportFileUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func (a *Analyzer) writeReportFile(parsed repo.ParsedURL, rev repo.Revision, markdown string) (string, error) {
	if a.reportsDir == "" {
		return "", nil
	}
	if err := os.MkdirAll(a.reportsDir, 0o755); err != nil {
		return "", err
	}
	shortSHA := rev.CommitSHA
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	name := fmt.Sprintf("%s_%s_%s.md", parsed.Owner, parsed.Name, shortSHA)
	name = reportFileUnsafe.ReplaceAllString(name, "_")
	path := filepath.Join(a.reportsDir, name)
	return path, os.WriteFile(path, []byte(markdown), 0o644)
}
