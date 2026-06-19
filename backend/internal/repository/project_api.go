package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrProjectNotFound = errors.New("project not found")

type Project struct {
	ID              string    `json:"id"`
	UserID          string    `json:"-"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Industry        *string   `json:"industry,omitempty"`
	Market          *string   `json:"market,omitempty"`
	TargetAudience  *string   `json:"target_audience,omitempty"`
	Goal            string    `json:"goal"`
	BudgetRange     *string   `json:"budget_range,omitempty"`
	StyleKeywords   []string  `json:"style_keywords"`
	SelectedEntries []string  `json:"selected_entries"`
	Status          string    `json:"status"`
	ErrorMessage    *string   `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ProjectBrief struct {
	ID               string   `json:"id"`
	Objective        string   `json:"objective"`
	Audience         string   `json:"audience"`
	Positioning      string   `json:"positioning"`
	ValueProposition string   `json:"value_proposition"`
	ToneOfVoice      string   `json:"tone_of_voice"`
	VisualDirection  string   `json:"visual_direction"`
	KeyMessages      []string `json:"key_messages"`
	Assumptions      []string `json:"assumptions"`
}

type Task struct {
	ID           string     `json:"id"`
	SkillType    string     `json:"skill_type"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type Deliverable struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	TaskID    *string   `json:"task_id,omitempty"`
	Entry     string    `json:"entry"`
	Surface   *string   `json:"surface,omitempty"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Format    string    `json:"format"`
	Content   string    `json:"content"`
	Version   int       `json:"version"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProjectDetail struct {
	Project Project
	Brief   *ProjectBrief
	Tasks   []Task
}

type ListProjectsParams struct {
	UserID   string
	Status   string
	Page     int
	PageSize int
}

func (s *PostgresStore) ListProjects(ctx context.Context, params ListProjectsParams) ([]Project, int, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var rows pgx.Rows
	var err error
	var total int
	if params.Status == "" {
		err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE user_id = $1`, params.UserID).Scan(&total)
		if err == nil {
			rows, err = s.db.Query(ctx, `
				SELECT id, user_id, name, type, industry, market, target_audience, goal,
					budget_range, COALESCE(style_keywords, '{}'), selected_entries, status, error_message,
					created_at, updated_at
				FROM projects
				WHERE user_id = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			`, params.UserID, pageSize, offset)
		}
	} else {
		err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE user_id = $1 AND status = $2`, params.UserID, params.Status).Scan(&total)
		if err == nil {
			rows, err = s.db.Query(ctx, `
				SELECT id, user_id, name, type, industry, market, target_audience, goal,
					budget_range, COALESCE(style_keywords, '{}'), selected_entries, status, error_message,
					created_at, updated_at
				FROM projects
				WHERE user_id = $1 AND status = $2
				ORDER BY created_at DESC
				LIMIT $3 OFFSET $4
			`, params.UserID, params.Status, pageSize, offset)
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", scrubDatabaseError(err))
	}
	defer rows.Close()

	projects := make([]Project, 0)
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read projects: %w", scrubDatabaseError(err))
	}
	return projects, total, nil
}

func (s *PostgresStore) GetProjectDetail(ctx context.Context, projectID string, userID string) (*ProjectDetail, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, user_id, name, type, industry, market, target_audience, goal,
			budget_range, COALESCE(style_keywords, '{}'), selected_entries, status, error_message,
			created_at, updated_at
		FROM projects
		WHERE id = $1 AND user_id = $2
	`, projectID, userID)
	project, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}

	brief, err := s.getProjectBrief(ctx, projectID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.listProjectTasks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &ProjectDetail{Project: project, Brief: brief, Tasks: tasks}, nil
}

func (s *PostgresStore) GetDeliverable(ctx context.Context, deliverableID string, userID string) (*Deliverable, error) {
	row := s.db.QueryRow(ctx, `
		SELECT d.id, d.project_id, d.task_id, d.entry, d.surface, d.type, d.title,
			d.format, COALESCE(d.content, ''), d.version, d.status, d.created_at, d.updated_at
		FROM deliverables d
		JOIN projects p ON p.id = d.project_id
		WHERE d.id = $1 AND p.user_id = $2
	`, deliverableID, userID)
	deliverable, err := scanDeliverable(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &deliverable, nil
}

func (s *PostgresStore) ListDeliverables(ctx context.Context, projectID string, userID string, entry string) ([]Deliverable, error) {
	var rows pgx.Rows
	var err error
	if entry == "" {
		rows, err = s.db.Query(ctx, `
			SELECT d.id, d.project_id, d.task_id, d.entry, d.surface, d.type, d.title,
				d.format, COALESCE(d.content, ''), d.version, d.status, d.created_at, d.updated_at
			FROM deliverables d
			JOIN projects p ON p.id = d.project_id
			WHERE d.project_id = $1 AND p.user_id = $2
			ORDER BY d.created_at ASC
		`, projectID, userID)
	} else {
		rows, err = s.db.Query(ctx, `
			SELECT d.id, d.project_id, d.task_id, d.entry, d.surface, d.type, d.title,
				d.format, COALESCE(d.content, ''), d.version, d.status, d.created_at, d.updated_at
			FROM deliverables d
			JOIN projects p ON p.id = d.project_id
			WHERE d.project_id = $1 AND p.user_id = $2 AND d.entry = $3
			ORDER BY d.created_at ASC
		`, projectID, userID, entry)
	}
	if err != nil {
		return nil, fmt.Errorf("list deliverables: %w", scrubDatabaseError(err))
	}
	defer rows.Close()

	deliverables := make([]Deliverable, 0)
	for rows.Next() {
		deliverable, err := scanDeliverable(rows)
		if err != nil {
			return nil, err
		}
		deliverables = append(deliverables, deliverable)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read deliverables: %w", scrubDatabaseError(err))
	}
	return deliverables, nil
}

func (s *PostgresStore) getProjectBrief(ctx context.Context, projectID string) (*ProjectBrief, error) {
	var brief ProjectBrief
	err := s.db.QueryRow(ctx, `
		SELECT id, objective, audience, positioning, value_proposition,
			tone_of_voice, visual_direction, COALESCE(key_messages, '{}'), COALESCE(assumptions, '{}')
		FROM project_briefs
		WHERE project_id = $1
	`, projectID).Scan(
		&brief.ID,
		&brief.Objective,
		&brief.Audience,
		&brief.Positioning,
		&brief.ValueProposition,
		&brief.ToneOfVoice,
		&brief.VisualDirection,
		&brief.KeyMessages,
		&brief.Assumptions,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project brief: %w", scrubDatabaseError(err))
	}
	return &brief, nil
}

func (s *PostgresStore) listProjectTasks(ctx context.Context, projectID string) ([]Task, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, skill_type, title, status, error_message, started_at, completed_at
		FROM tasks
		WHERE project_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project tasks: %w", scrubDatabaseError(err))
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		var startedAt pgtype.Timestamptz
		var completedAt pgtype.Timestamptz
		if err := rows.Scan(&task.ID, &task.SkillType, &task.Title, &task.Status, &task.ErrorMessage, &startedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", scrubDatabaseError(err))
		}
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
	return tasks, nil
}

type projectScanner interface {
	Scan(dest ...any) error
}

func scanProject(row projectScanner) (Project, error) {
	var project Project
	if err := row.Scan(
		&project.ID,
		&project.UserID,
		&project.Name,
		&project.Type,
		&project.Industry,
		&project.Market,
		&project.TargetAudience,
		&project.Goal,
		&project.BudgetRange,
		&project.StyleKeywords,
		&project.SelectedEntries,
		&project.Status,
		&project.ErrorMessage,
		&project.CreatedAt,
		&project.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, err
		}
		return Project{}, fmt.Errorf("scan project: %w", scrubDatabaseError(err))
	}
	return project, nil
}

func scanDeliverable(row projectScanner) (Deliverable, error) {
	var deliverable Deliverable
	if err := row.Scan(
		&deliverable.ID,
		&deliverable.ProjectID,
		&deliverable.TaskID,
		&deliverable.Entry,
		&deliverable.Surface,
		&deliverable.Type,
		&deliverable.Title,
		&deliverable.Format,
		&deliverable.Content,
		&deliverable.Version,
		&deliverable.Status,
		&deliverable.CreatedAt,
		&deliverable.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Deliverable{}, err
		}
		return Deliverable{}, fmt.Errorf("scan deliverable: %w", scrubDatabaseError(err))
	}
	return deliverable, nil
}
