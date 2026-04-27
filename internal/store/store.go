package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) UpsertProject(ctx context.Context, input ProjectInput) (*Project, bool, error) {
	project, err := s.GetProjectByNormalizedURL(ctx, input.NormalizedURL)
	if err == nil {
		return project, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects (repo_url, normalized_url, owner, name, status) VALUES (?, ?, ?, ?, ?)`,
		input.RepoURL, input.NormalizedURL, input.Owner, input.Name, StatusQueued)
	if err != nil {
		return nil, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, false, err
	}
	project, err = s.GetProject(ctx, id)
	return project, true, err
}

func (s *Store) GetProject(ctx context.Context, id int64) (*Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, repo_url, normalized_url, owner, name, COALESCE(default_branch, ''), COALESCE(commit_sha, ''), status, created_at, updated_at FROM projects WHERE id = ?`, id)
	return scanProject(row)
}

func (s *Store) GetProjectByNormalizedURL(ctx context.Context, normalizedURL string) (*Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, repo_url, normalized_url, owner, name, COALESCE(default_branch, ''), COALESCE(commit_sha, ''), status, created_at, updated_at FROM projects WHERE normalized_url = ?`, normalizedURL)
	return scanProject(row)
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, repo_url, normalized_url, owner, name, COALESCE(default_branch, ''), COALESCE(commit_sha, ''), status, created_at, updated_at FROM projects ORDER BY updated_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		has, _ := s.HasReport(ctx, project.ID, project.CommitSHA)
		project.HasReport = has
		run, _ := s.LatestRun(ctx, project.ID)
		project.CurrentRun = run
		projects = append(projects, *project)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateProjectRevision(ctx context.Context, id int64, branch, commitSHA, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET default_branch = ?, commit_sha = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, branch, commitSHA, status, id)
	return err
}

func (s *Store) SetProjectStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE projects SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, id)
	return err
}

func (s *Store) RecoverInterruptedRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE analysis_runs
		SET status = ?, step = 'error', progress = 100, message = '上次服务重启，中断的分析已停止', error = '服务重启导致分析中断', finished_at = CURRENT_TIMESTAMP
		WHERE status IN (?, ?) AND finished_at IS NULL`, StatusFailed, StatusQueued, StatusRunning)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE projects
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE status IN (?, ?)`, StatusFailed, StatusQueued, StatusRunning)
	return err
}

func (s *Store) CreateRun(ctx context.Context, projectID int64) (*Run, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO analysis_runs (project_id, status, step, progress, message) VALUES (?, ?, ?, ?, ?)`,
		projectID, StatusQueued, StatusQueued, 0, "等待分析")
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetRun(ctx, id)
}

func (s *Store) GetRun(ctx context.Context, id int64) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, project_id, status, step, progress, message, error, started_at, finished_at FROM analysis_runs WHERE id = ?`, id)
	return scanRun(row)
}

func (s *Store) LatestRun(ctx context.Context, projectID int64) (*Run, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, project_id, status, step, progress, message, error, started_at, finished_at FROM analysis_runs WHERE project_id = ? ORDER BY id DESC LIMIT 1`, projectID)
	return scanRun(row)
}

func (s *Store) UpdateRun(ctx context.Context, runID int64, status, step string, progress int, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE analysis_runs SET status = ?, step = ?, progress = ?, message = ? WHERE id = ?`, status, step, progress, message, runID)
	return err
}

func (s *Store) FinishRun(ctx context.Context, runID int64, status, step, message, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE analysis_runs SET status = ?, step = ?, progress = ?, message = ?, error = ?, finished_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, step, 100, message, errMsg, runID)
	return err
}

func (s *Store) SaveReport(ctx context.Context, projectID int64, commitSHA, markdown, techStackJSON, treeJSON string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO analysis_reports (project_id, commit_sha, markdown, tech_stack_json, tree_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id, commit_sha) DO UPDATE SET markdown = excluded.markdown, tech_stack_json = excluded.tech_stack_json, tree_json = excluded.tree_json, created_at = CURRENT_TIMESTAMP`,
		projectID, commitSHA, markdown, techStackJSON, treeJSON)
	return err
}

func (s *Store) GetReport(ctx context.Context, projectID int64, commitSHA string) (string, error) {
	var markdown string
	err := s.db.QueryRowContext(ctx, `SELECT markdown FROM analysis_reports WHERE project_id = ? AND commit_sha = ?`, projectID, commitSHA).Scan(&markdown)
	return markdown, err
}

func (s *Store) HasReport(ctx context.Context, projectID int64, commitSHA string) (bool, error) {
	if commitSHA == "" {
		return false, nil
	}
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_reports WHERE project_id = ? AND commit_sha = ?)`, projectID, commitSHA).Scan(&exists)
	return exists == 1, err
}

func (s *Store) ReplaceFileIndex(ctx context.Context, projectID int64, commitSHA string, files []FileRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM file_index WHERE project_id = ? AND commit_sha = ?`, projectID, commitSHA); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO file_index (project_id, commit_sha, path, language, size, hash, summary, content) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, file := range files {
		if _, err := stmt.ExecContext(ctx, projectID, commitSHA, file.Path, file.Language, file.Size, file.Hash, file.Summary, file.Content); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListFiles(ctx context.Context, projectID int64, commitSHA string, limit int) ([]FileRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT project_id, commit_sha, path, language, size, hash, summary, '' FROM file_index WHERE project_id = ? AND commit_sha = ? ORDER BY path LIMIT ?`, projectID, commitSHA, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) ReadFile(ctx context.Context, projectID int64, commitSHA, path string) (*FileRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT project_id, commit_sha, path, language, size, hash, summary, content FROM file_index WHERE project_id = ? AND commit_sha = ? AND path = ?`, projectID, commitSHA, path)
	var file FileRecord
	err := row.Scan(&file.ProjectID, &file.CommitSHA, &file.Path, &file.Language, &file.Size, &file.Hash, &file.Summary, &file.Content)
	return &file, err
}

func (s *Store) SearchFiles(ctx context.Context, projectID int64, commitSHA, query string, limit int) ([]FileRecord, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	like := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT project_id, commit_sha, path, language, size, hash, summary, content FROM file_index WHERE project_id = ? AND commit_sha = ? AND (path LIKE ? OR content LIKE ?) ORDER BY path LIMIT ?`, projectID, commitSHA, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFiles(rows)
}

func (s *Store) SaveChatMessage(ctx context.Context, projectID int64, role, content string) error {
	sessionID, err := s.ensureSession(ctx, projectID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO chat_messages (session_id, role, content) VALUES (?, ?, ?)`, sessionID, role, content)
	return err
}

func (s *Store) RecentChatMessages(ctx context.Context, projectID int64, limit int) ([]ChatMessage, error) {
	sessionID, err := s.ensureSession(ctx, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT role, content FROM chat_messages WHERE session_id = ? ORDER BY id DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reversed []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		reversed = append(reversed, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

type ChatMessage struct {
	Role    string
	Content string
}

func (s *Store) ensureSession(ctx context.Context, projectID int64) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM chat_sessions WHERE project_id = ? ORDER BY id DESC LIMIT 1`, projectID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO chat_sessions (project_id) VALUES (?)`, projectID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (*Project, error) {
	var project Project
	err := row.Scan(&project.ID, &project.RepoURL, &project.NormalizedURL, &project.Owner, &project.Name, &project.DefaultBranch, &project.CommitSHA, &project.Status, &project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func scanRun(row scanner) (*Run, error) {
	var run Run
	err := row.Scan(&run.ID, &run.ProjectID, &run.Status, &run.Step, &run.Progress, &run.Message, &run.Error, &run.StartedAt, &run.FinishedAt)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func scanFiles(rows *sql.Rows) ([]FileRecord, error) {
	var files []FileRecord
	for rows.Next() {
		var file FileRecord
		if err := rows.Scan(&file.ProjectID, &file.CommitSHA, &file.Path, &file.Language, &file.Size, &file.Hash, &file.Summary, &file.Content); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}
	return files, nil
}
