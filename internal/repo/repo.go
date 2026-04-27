package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"project-helper/internal/store"
)

const maxIndexedFileBytes = 200_000

var ownerRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type ParsedURL struct {
	Original   string
	Normalized string
	Owner      string
	Name       string
}

type Revision struct {
	DefaultBranch string
	CommitSHA     string
	CloneURL      string
}

type Service struct {
	reposDir string
}

func NewService(reposDir string) *Service {
	return &Service{reposDir: reposDir}
}

func ParseGitHubURL(raw string) (ParsedURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedURL{}, errors.New("请输入 GitHub 仓库地址")
	}

	var ownerRepo string
	if strings.HasPrefix(raw, "git@github.com:") {
		ownerRepo = strings.TrimSuffix(strings.TrimPrefix(raw, "git@github.com:"), ".git")
	} else if !strings.Contains(raw, "://") && ownerRepoPattern.MatchString(strings.TrimSuffix(raw, ".git")) {
		ownerRepo = strings.TrimSuffix(raw, ".git")
	} else {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return ParsedURL{}, errors.New("仓库地址格式不正确")
		}
		if strings.ToLower(parsed.Host) != "github.com" {
			return ParsedURL{}, errors.New("v1 仅支持公开 GitHub 仓库")
		}
		parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
		if len(parts) < 2 {
			return ParsedURL{}, errors.New("GitHub 地址需要包含 owner/repo")
		}
		ownerRepo = parts[0] + "/" + parts[1]
	}

	if !ownerRepoPattern.MatchString(ownerRepo) {
		return ParsedURL{}, errors.New("仓库路径只能是 owner/repo")
	}
	parts := strings.Split(ownerRepo, "/")
	normalized := fmt.Sprintf("https://github.com/%s/%s", parts[0], strings.TrimSuffix(parts[1], ".git"))
	return ParsedURL{
		Original:   raw,
		Normalized: normalized,
		Owner:      parts[0],
		Name:       strings.TrimSuffix(parts[1], ".git"),
	}, nil
}

func (s *Service) ResolveRevision(ctx context.Context, parsed ParsedURL) (Revision, error) {
	cloneURL := parsed.Normalized + ".git"
	remote := git.NewRemote(memory.NewStorage(), &gitconfig.RemoteConfig{
		Name: git.DefaultRemoteName,
		URLs: []string{cloneURL},
	})
	refs, err := remote.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return Revision{}, fmt.Errorf("无法读取远程仓库信息，请确认仓库公开且地址正确")
	}
	branch, sha, err := revisionFromRefs(refs)
	if err != nil {
		return Revision{}, err
	}
	return Revision{DefaultBranch: branch, CommitSHA: sha, CloneURL: cloneURL}, nil
}

func (s *Service) Clone(ctx context.Context, parsed ParsedURL, rev Revision) (string, error) {
	target := filepath.Join(s.reposDir, parsed.Owner, parsed.Name, rev.CommitSHA)
	if _, err := os.Stat(filepath.Join(target, ".git")); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	_ = os.RemoveAll(target)
	_, err := git.PlainCloneContext(ctx, target, false, &git.CloneOptions{
		URL:           rev.CloneURL,
		ReferenceName: plumbing.NewBranchReferenceName(rev.DefaultBranch),
		SingleBranch:  true,
		Depth:         1,
		Tags:          git.NoTags,
	})
	if err != nil {
		return "", fmt.Errorf("克隆失败: %w", err)
	}
	return target, nil
}

func revisionFromRefs(refs []*plumbing.Reference) (string, string, error) {
	if len(refs) == 0 {
		return "", "", errors.New("无法解析仓库 HEAD commit")
	}

	byName := make(map[plumbing.ReferenceName]*plumbing.Reference, len(refs))
	var branches []plumbing.ReferenceName
	var headTarget plumbing.ReferenceName
	var headHash plumbing.Hash
	for _, ref := range refs {
		byName[ref.Name()] = ref
		if ref.Name().IsBranch() {
			branches = append(branches, ref.Name())
		}
		if ref.Name() != plumbing.HEAD {
			continue
		}
		switch ref.Type() {
		case plumbing.SymbolicReference:
			headTarget = ref.Target()
		case plumbing.HashReference:
			headHash = ref.Hash()
		}
	}

	if headTarget != "" {
		if ref := byName[headTarget]; ref != nil && ref.Hash() != plumbing.ZeroHash {
			return branchName(headTarget), ref.Hash().String(), nil
		}
	}
	if headHash != plumbing.ZeroHash {
		return fallbackBranch(branches), headHash.String(), nil
	}

	for _, name := range []plumbing.ReferenceName{
		plumbing.NewBranchReferenceName("main"),
		plumbing.NewBranchReferenceName("master"),
	} {
		if ref := byName[name]; ref != nil && ref.Hash() != plumbing.ZeroHash {
			return branchName(name), ref.Hash().String(), nil
		}
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].String() < branches[j].String() })
	for _, name := range branches {
		if ref := byName[name]; ref != nil && ref.Hash() != plumbing.ZeroHash {
			return branchName(name), ref.Hash().String(), nil
		}
	}
	return "", "", errors.New("无法解析仓库 HEAD commit")
}

func branchName(name plumbing.ReferenceName) string {
	if name.IsBranch() {
		return name.Short()
	}
	return strings.TrimPrefix(name.String(), "refs/heads/")
}

func fallbackBranch(branches []plumbing.ReferenceName) string {
	for _, name := range branches {
		short := branchName(name)
		if short == "main" || short == "master" {
			return short
		}
	}
	if len(branches) > 0 {
		sort.Slice(branches, func(i, j int) bool { return branches[i].String() < branches[j].String() })
		return branchName(branches[0])
	}
	return "main"
}

func IndexFiles(ctx context.Context, root string, projectID int64, commitSHA string) ([]store.FileRecord, Tree, error) {
	var files []store.FileRecord
	tree := Tree{Name: filepath.Base(root), Path: ".", Type: "dir"}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldSkip(entry, rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			addTreeNode(&tree, rel, "dir")
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxIndexedFileBytes {
			return nil
		}
		contentBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if isBinary(contentBytes) {
			return nil
		}
		content := string(contentBytes)
		hashBytes := sha256.Sum256(contentBytes)
		file := store.FileRecord{
			ProjectID: projectID,
			CommitSHA: commitSHA,
			Path:      rel,
			Language:  languageFor(rel),
			Size:      info.Size(),
			Hash:      hex.EncodeToString(hashBytes[:]),
			Summary:   summarizeFile(rel, content),
			Content:   content,
		}
		files = append(files, file)
		addTreeNode(&tree, rel, "file")
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sortTree(&tree)
	return files, tree, err
}

type Tree struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Children []Tree `json:"children,omitempty"`
}

func shouldSkip(entry fs.DirEntry, rel string) bool {
	name := entry.Name()
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
		"target": true, ".next": true, ".nuxt": true, ".idea": true, ".vscode": true,
		"coverage": true, "__pycache__": true,
	}
	if entry.IsDir() && skipDirs[name] {
		return true
	}
	if strings.HasPrefix(name, ".") && name != ".github" && entry.IsDir() {
		return true
	}
	skipSuffixes := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf", ".zip", ".tar", ".gz", ".lock"}
	lower := strings.ToLower(rel)
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func isBinary(content []byte) bool {
	for i, b := range content {
		if i > 8000 {
			break
		}
		if b == 0 {
			return true
		}
	}
	return false
}

func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "Go"
	case ".js", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts":
		return "TypeScript"
	case ".vue":
		return "Vue"
	case ".py":
		return "Python"
	case ".java":
		return "Java"
	case ".rs":
		return "Rust"
	case ".md", ".mdx":
		return "Markdown"
	case ".json":
		return "JSON"
	case ".yml", ".yaml":
		return "YAML"
	case ".css":
		return "CSS"
	case ".html":
		return "HTML"
	default:
		return "Text"
	}
}

func summarizeFile(path, content string) string {
	lines := strings.Split(content, "\n")
	var interesting []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "package ") || strings.HasPrefix(trimmed, "func ") ||
			strings.HasPrefix(trimmed, "type ") || strings.HasPrefix(trimmed, "export ") ||
			strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "def ") ||
			strings.HasPrefix(trimmed, "# ") {
			interesting = append(interesting, trimmed)
		}
		if len(interesting) >= 5 {
			break
		}
	}
	if len(interesting) == 0 {
		return fmt.Sprintf("%s，约 %d 行", path, len(lines))
	}
	return strings.Join(interesting, " | ")
}

func addTreeNode(root *Tree, rel, nodeType string) {
	parts := strings.Split(rel, "/")
	current := root
	for i, part := range parts {
		nodePath := strings.Join(parts[:i+1], "/")
		found := -1
		for idx := range current.Children {
			if current.Children[idx].Name == part {
				found = idx
				break
			}
		}
		t := "dir"
		if i == len(parts)-1 {
			t = nodeType
		}
		if found == -1 {
			current.Children = append(current.Children, Tree{Name: part, Path: nodePath, Type: t})
			found = len(current.Children) - 1
		}
		current = &current.Children[found]
	}
}

func sortTree(tree *Tree) {
	sort.Slice(tree.Children, func(i, j int) bool {
		if tree.Children[i].Type != tree.Children[j].Type {
			return tree.Children[i].Type == "dir"
		}
		return tree.Children[i].Name < tree.Children[j].Name
	})
	for i := range tree.Children {
		sortTree(&tree.Children[i])
	}
}

func SafeRelativePath(path string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", errors.New("文件路径越界")
	}
	return cleaned, nil
}
