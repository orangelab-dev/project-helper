package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"project-helper/internal/ai"
	"project-helper/internal/repo"
	"project-helper/internal/store"
)

type Event struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Name string `json:"name,omitempty"`
}

type Agent struct {
	store *store.Store
	ai    ai.Client
}

func New(store *store.Store, aiClient ai.Client) *Agent {
	return &Agent{store: store, ai: aiClient}
}

func (a *Agent) Answer(ctx context.Context, project *store.Project, question string) (<-chan Event, <-chan error) {
	events := make(chan Event, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		question = strings.TrimSpace(question)
		if question == "" {
			errs <- fmt.Errorf("请输入问题")
			return
		}
		if project.CommitSHA == "" || project.Status != store.StatusCompleted {
			errs <- fmt.Errorf("项目还没有完成分析")
			return
		}

		_ = a.store.SaveChatMessage(ctx, project.ID, "user", question)
		contextText, err := a.gatherContext(ctx, project, question, events)
		if err != nil {
			errs <- err
			return
		}
		history, _ := a.store.RecentChatMessages(ctx, project.ID, 8)
		messages := []ai.Message{{Role: "system", Content: systemPrompt()}}
		for _, msg := range history {
			messages = append(messages, ai.Message{Role: msg.Role, Content: msg.Content})
		}
		messages = append(messages, ai.Message{Role: "user", Content: fmt.Sprintf("用户问题：%s\n\n你通过工具查到的源码上下文：\n%s", question, contextText)})
		_ = ai.ToEinoMessages(messages)

		stream, streamErrs := a.ai.Stream(ctx, messages)
		var answer strings.Builder
		for event := range stream {
			if event.Type == "token" {
				answer.WriteString(event.Data)
				events <- Event{Type: "token", Data: event.Data}
			}
		}
		for err := range streamErrs {
			if err != nil {
				errs <- err
				return
			}
		}
		_ = a.store.SaveChatMessage(ctx, project.ID, "assistant", answer.String())
		events <- Event{Type: "done"}
	}()
	return events, errs
}

func (a *Agent) gatherContext(ctx context.Context, project *store.Project, question string, events chan<- Event) (string, error) {
	events <- Event{Type: "tool_call", Name: "search_code", Data: question}
	files, err := a.store.SearchFiles(ctx, project.ID, project.CommitSHA, question, 6)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		events <- Event{Type: "tool_result", Name: "search_code", Data: "没有直接命中，改为读取项目文件列表"}
		events <- Event{Type: "tool_call", Name: "list_files"}
		files, err = a.store.ListFiles(ctx, project.ID, project.CommitSHA, 12)
		if err != nil {
			return "", err
		}
	} else {
		payload, _ := json.Marshal(pathsOf(files))
		events <- Event{Type: "tool_result", Name: "search_code", Data: string(payload)}
	}

	var b strings.Builder
	for _, file := range files {
		path, err := repo.SafeRelativePath(file.Path)
		if err != nil {
			continue
		}
		events <- Event{Type: "tool_call", Name: "read_file", Data: path}
		record, err := a.store.ReadFile(ctx, project.ID, project.CommitSHA, path)
		if err != nil {
			continue
		}
		content := record.Content
		if len(content) > 6000 {
			content = content[:6000] + "\n...（内容过长，已截断）"
		}
		fmt.Fprintf(&b, "\n文件：%s\n语言：%s\n摘要：%s\n内容：\n%s\n", record.Path, record.Language, record.Summary, content)
		events <- Event{Type: "tool_result", Name: "read_file", Data: path}
	}

	report, err := a.store.GetReport(ctx, project.ID, project.CommitSHA)
	if err == nil {
		if len(report) > 5000 {
			report = report[:5000] + "\n...（报告过长，已截断）"
		}
		events <- Event{Type: "tool_call", Name: "get_report_section"}
		fmt.Fprintf(&b, "\n已有分析报告节选：\n%s\n", report)
		events <- Event{Type: "tool_result", Name: "get_report_section", Data: "已读取报告节选"}
	}
	return b.String(), nil
}

func pathsOf(files []store.FileRecord) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func systemPrompt() string {
	return `你是 project-helper 的源码问答 Agent。请用中文回答，面向初学者，先给结论，再解释依据。你已经拥有读取文件、搜索代码、查看目录和读取报告的工具结果；回答必须引用相关文件路径。不能编造不存在的文件或函数。`
}
