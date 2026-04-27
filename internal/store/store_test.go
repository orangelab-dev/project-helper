package store

import (
	"path/filepath"
	"testing"

	appdb "project-helper/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	database, err := appdb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := appdb.Migrate(database); err != nil {
		t.Fatal(err)
	}
	return New(database)
}

func TestReportCacheHit(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	project, _, err := st.UpsertProject(ctx, ProjectInput{
		RepoURL:       "https://github.com/example/demo",
		NormalizedURL: "https://github.com/example/demo",
		Owner:         "example",
		Name:          "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateProjectRevision(ctx, project.ID, "main", "abc123", StatusRunning); err != nil {
		t.Fatal(err)
	}
	has, err := st.HasReport(ctx, project.ID, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Fatal("report should not exist yet")
	}
	if err := st.SaveReport(ctx, project.ID, "abc123", "# report", `["Go"]`, `{}`); err != nil {
		t.Fatal(err)
	}
	has, err = st.HasReport(ctx, project.ID, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected report cache hit")
	}
}

func TestFileIndexSearchAndRead(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	project, _, err := st.UpsertProject(ctx, ProjectInput{
		RepoURL:       "https://github.com/example/demo",
		NormalizedURL: "https://github.com/example/demo",
		Owner:         "example",
		Name:          "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	files := []FileRecord{{
		ProjectID: project.ID,
		CommitSHA: "abc123",
		Path:      "internal/server.go",
		Language:  "Go",
		Size:      42,
		Hash:      "hash",
		Summary:   "func main",
		Content:   "package internal\nfunc Serve() {}",
	}}
	if err := st.ReplaceFileIndex(ctx, project.ID, "abc123", files); err != nil {
		t.Fatal(err)
	}
	results, err := st.SearchFiles(ctx, project.ID, "abc123", "Serve", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "internal/server.go" {
		t.Fatalf("unexpected results: %+v", results)
	}
	file, err := st.ReadFile(ctx, project.ID, "abc123", "internal/server.go")
	if err != nil {
		t.Fatal(err)
	}
	if file.Content == "" {
		t.Fatal("expected indexed content")
	}
}

func TestRecoverInterruptedRuns(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	project, _, err := st.UpsertProject(ctx, ProjectInput{
		RepoURL:       "https://github.com/example/demo",
		NormalizedURL: "https://github.com/example/demo",
		Owner:         "example",
		Name:          "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(ctx, run.ID, StatusRunning, "cloning", 25, "running"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetProjectStatus(ctx, project.ID, StatusRunning); err != nil {
		t.Fatal(err)
	}
	if err := st.RecoverInterruptedRuns(ctx); err != nil {
		t.Fatal(err)
	}
	recoveredRun, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredRun.Status != StatusFailed || recoveredRun.Error == "" {
		t.Fatalf("unexpected recovered run: %+v", recoveredRun)
	}
	recoveredProject, err := st.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredProject.Status != StatusFailed {
		t.Fatalf("project status = %q", recoveredProject.Status)
	}
}
