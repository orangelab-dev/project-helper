package store

import "time"

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

type Project struct {
	ID             int64     `json:"id"`
	RepoURL        string    `json:"repo_url"`
	NormalizedURL  string    `json:"normalized_url"`
	Owner          string    `json:"owner"`
	Name           string    `json:"name"`
	DefaultBranch  string    `json:"default_branch"`
	CommitSHA      string    `json:"commit_sha"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	HasReport      bool      `json:"has_report"`
	CurrentRun     *Run      `json:"current_run,omitempty"`
	ReportMarkdown string    `json:"-"`
}

type Run struct {
	ID         int64      `json:"id"`
	ProjectID  int64      `json:"project_id"`
	Status     string     `json:"status"`
	Step       string     `json:"step"`
	Progress   int        `json:"progress"`
	Message    string     `json:"message"`
	Error      string     `json:"error"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type FileRecord struct {
	ProjectID int64  `json:"project_id"`
	CommitSHA string `json:"commit_sha"`
	Path      string `json:"path"`
	Language  string `json:"language"`
	Size      int64  `json:"size"`
	Hash      string `json:"hash"`
	Summary   string `json:"summary"`
	Content   string `json:"content,omitempty"`
}

type ProjectInput struct {
	RepoURL       string
	NormalizedURL string
	Owner         string
	Name          string
}
