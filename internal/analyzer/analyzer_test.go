package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"project-helper/internal/repo"
)

func TestWriteReportFile(t *testing.T) {
	dir := t.TempDir()
	an := &Analyzer{reportsDir: dir}
	path, err := an.writeReportFile(repo.ParsedURL{
		Owner: "openai",
		Name:  "project-helper",
	}, repo.Revision{CommitSHA: "abcdef1234567890"}, "# Report")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "openai_project-helper_abcdef123456.md" {
		t.Fatalf("unexpected report path %q", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Report" {
		t.Fatalf("content = %q", string(content))
	}
}
