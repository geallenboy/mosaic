package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mosaic-app/mosaic/backend/internal/orchestrator"
	"github.com/mosaic-app/mosaic/backend/internal/skill"
)

const localDevOwnerEmail = "local-dev@mosaic.local"

type PostgresStore struct {
	db *pgxpool.Pool
}

type CreateProjectParams struct {
	Name            string
	Type            string
	Industry        string
	Market          string
	TargetAudience  string
	Goal            string
	BudgetRange     string
	StyleKeywords   []string
	SelectedEntries []string
}

type deliverableRecord struct {
	Entry   string
	Type    string
	Title   string
	Format  string
	Content string
	Status  string
}

func OpenPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", scrubDatabaseError(err))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", scrubDatabaseError(err))
	}
	return pool, nil
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateProject(ctx context.Context, params CreateProjectParams) (string, error) {
	ownerID, err := s.ensureLocalDevOwner(ctx)
	if err != nil {
		return "", err
	}

	projectType := params.Type
	if projectType == "" {
		projectType = "custom"
	}

	var projectID string
	err = s.db.QueryRow(ctx, `
		INSERT INTO projects (
			user_id, name, type, industry, market, target_audience, goal,
			budget_range, style_keywords, selected_entries, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'running')
		RETURNING id
	`, ownerID, params.Name, projectType, params.Industry, params.Market, params.TargetAudience,
		params.Goal, params.BudgetRange, params.StyleKeywords, params.SelectedEntries).Scan(&projectID)
	if err != nil {
		return "", fmt.Errorf("create project: %w", scrubDatabaseError(err))
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO project_briefs (
			project_id, objective, audience, positioning, value_proposition,
			tone_of_voice, visual_direction
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, projectID, params.Goal, params.TargetAudience, params.Name, params.Goal,
		"真诚、专业、有温度", "简洁现代")
	if err != nil {
		return "", fmt.Errorf("create project brief: %w", scrubDatabaseError(err))
	}

	return projectID, nil
}

func (s *PostgresStore) InitTasks(ctx context.Context, projectID string, tasks []orchestrator.TaskStatus) error {
	for _, task := range tasks {
		_, err := s.db.Exec(ctx, `
			INSERT INTO tasks (project_id, skill_type, title, status)
			VALUES ($1, $2, $3, $4)
		`, projectID, string(task.SkillType), taskTitle(task.SkillType), task.Status)
		if err != nil {
			return fmt.Errorf("init task: %w", scrubDatabaseError(err))
		}
	}
	return nil
}

func (s *PostgresStore) UpdateTaskStatus(ctx context.Context, projectID string, task orchestrator.TaskStatus) error {
	_, err := s.db.Exec(ctx, `
		UPDATE tasks
		SET status = $3,
			error_message = NULLIF($4, ''),
			started_at = COALESCE($5, started_at),
			completed_at = COALESCE($6, completed_at),
			updated_at = NOW()
		WHERE project_id = $1 AND skill_type = $2
	`, projectID, string(task.SkillType), task.Status, task.ErrorMsg, task.StartedAt, task.CompletedAt)
	if err != nil {
		return fmt.Errorf("update task status: %w", scrubDatabaseError(err))
	}
	return nil
}

func (s *PostgresStore) SaveDeliverable(ctx context.Context, projectID string, output *skill.Output) error {
	record := deliverableFromOutput(output)
	_, err := s.db.Exec(ctx, `
		INSERT INTO deliverables (
			project_id, task_id, entry, type, title, format, content, status
		)
		VALUES (
			$1,
			(SELECT id FROM tasks WHERE project_id = $1 AND skill_type = $2 LIMIT 1),
			$3, $4, $5, $6, $7, $8
		)
	`, projectID, string(output.SkillType), record.Entry, record.Type, record.Title,
		record.Format, record.Content, record.Status)
	if err != nil {
		return fmt.Errorf("save deliverable: %w", scrubDatabaseError(err))
	}
	return nil
}

func (s *PostgresStore) UpdateProjectStatus(ctx context.Context, projectID string, phase orchestrator.Phase, _ int, errorMsg string) error {
	status := projectStatusFromPhase(phase)
	_, err := s.db.Exec(ctx, `
		UPDATE projects
		SET status = $2,
			error_message = NULLIF($3, ''),
			updated_at = NOW()
		WHERE id = $1
	`, projectID, status, errorMsg)
	if err != nil {
		return fmt.Errorf("update project status: %w", scrubDatabaseError(err))
	}
	return nil
}

func (s *PostgresStore) GetStatus(ctx context.Context, projectID string) (*orchestrator.ProjectStatus, error) {
	var projectStatus string
	err := s.db.QueryRow(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&projectStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, orchestrator.ErrStatusNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get project status: %w", scrubDatabaseError(err))
	}

	rows, err := s.db.Query(ctx, `
		SELECT skill_type, status, COALESCE(error_message, ''), started_at, completed_at
		FROM tasks
		WHERE project_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("get tasks: %w", scrubDatabaseError(err))
	}
	defer rows.Close()

	tasks := make([]orchestrator.TaskStatus, 0)
	for rows.Next() {
		var task orchestrator.TaskStatus
		var skillType string
		var startedAt pgtype.Timestamptz
		var completedAt pgtype.Timestamptz
		if err := rows.Scan(&skillType, &task.Status, &task.ErrorMsg, &startedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", scrubDatabaseError(err))
		}
		task.SkillType = skill.Type(skillType)
		if startedAt.Valid {
			t := startedAt.Time
			task.StartedAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			task.CompletedAt = &t
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tasks: %w", scrubDatabaseError(err))
	}

	return &orchestrator.ProjectStatus{
		ProjectID: projectID,
		Phase:     phaseFromProjectStatus(projectStatus, tasks),
		Progress:  progressFromTasks(projectStatus, tasks),
		Tasks:     tasks,
	}, nil
}

func (s *PostgresStore) ensureLocalDevOwner(ctx context.Context) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (email, display_name, password_hash, role, status)
		VALUES ($1, 'Local Dev User', 'local-dev-disabled', 'user', 'active')
		ON CONFLICT (email) DO UPDATE SET updated_at = NOW()
		RETURNING id
	`, localDevOwnerEmail).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("ensure local owner: %w", scrubDatabaseError(err))
	}
	return userID, nil
}

func deliverableFromOutput(output *skill.Output) deliverableRecord {
	return deliverableRecord{
		Entry:   output.EntryType,
		Type:    string(output.SkillType),
		Title:   taskTitle(output.SkillType) + " 输出",
		Format:  string(output.Format),
		Content: output.Content,
		Status:  "done",
	}
}

func taskTitle(skillType skill.Type) string {
	switch skillType {
	case skill.TypeResearch:
		return "Research Skill"
	case skill.TypeStrategy:
		return "Strategy Skill"
	case skill.TypeCopy:
		return "Copy Skill"
	case skill.TypeDeck:
		return "Deck Skill"
	case skill.TypeVisual:
		return "Visual Skill"
	case skill.TypeWeb:
		return "Web Skill"
	case skill.TypeVideo:
		return "Video Skill"
	case skill.TypeOps:
		return "Ops Skill"
	case skill.TypeDev:
		return "Dev Skill"
	default:
		return strings.ReplaceAll(string(skillType), "_", " ") + " Skill"
	}
}

func projectStatusFromPhase(phase orchestrator.Phase) string {
	switch phase {
	case orchestrator.PhaseDone:
		return "done"
	case orchestrator.PhaseFailed:
		return "failed"
	default:
		return "running"
	}
}

func phaseFromProjectStatus(projectStatus string, tasks []orchestrator.TaskStatus) orchestrator.Phase {
	switch projectStatus {
	case "done":
		return orchestrator.PhaseDone
	case "failed":
		return orchestrator.PhaseFailed
	default:
		if len(tasks) == 0 {
			return orchestrator.PhaseBriefGeneration
		}
		return orchestrator.PhaseSkillExecution
	}
}

func progressFromTasks(projectStatus string, tasks []orchestrator.TaskStatus) int {
	switch projectStatus {
	case "done":
		return 100
	case "failed":
		return 0
	}
	if len(tasks) == 0 {
		return 0
	}
	completed := 0
	for _, task := range tasks {
		if task.Status == "done" || task.Status == "failed" {
			completed++
		}
	}
	return (completed * 100) / len(tasks)
}

func scrubDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "://") {
		return errors.New("database operation failed")
	}
	return err
}
