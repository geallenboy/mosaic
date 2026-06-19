package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mosaic-app/mosaic/backend/internal/auth"
	"github.com/mosaic-app/mosaic/backend/internal/orchestrator"
	"github.com/mosaic-app/mosaic/backend/internal/repository"
	"github.com/mosaic-app/mosaic/backend/internal/skill"
)

func TestCreateAndStartUsesRepositoryProjectID(t *testing.T) {
	creator := &fakeProjectCreator{projectID: "db-project-id"}
	starter := &fakeProjectStarter{}
	handler := NewProjectHandler(starter, creator)

	body := bytes.NewBufferString(`{
		"name":"Fresh Bowl",
		"type":"store_opening",
		"industry":"restaurant",
		"market":"Shenzhen",
		"target_audience":"office workers",
		"goal":"Launch a healthy lunch store opening campaign",
		"selected_entries":["documents"]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/start", body)
	rec := httptest.NewRecorder()

	handler.CreateAndStart(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if starter.projectID != "db-project-id" {
		t.Fatalf("expected starter to receive database project ID, got %q", starter.projectID)
	}
	if starter.brief == nil || starter.brief.ProjectID != "db-project-id" {
		t.Fatalf("expected brief to use database project ID, got %#v", starter.brief)
	}

	var resp struct {
		Data struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.ProjectID != "db-project-id" {
		t.Fatalf("expected response project ID from repository, got %q", resp.Data.ProjectID)
	}
}

func TestCreateAndStartPassesCurrentUserToRepository(t *testing.T) {
	creator := &fakeProjectCreator{projectID: "db-project-id"}
	starter := &fakeProjectStarter{}
	handler := NewProjectHandler(starter, creator)

	body := bytes.NewBufferString(`{
		"name":"Fresh Bowl",
		"type":"store_opening",
		"goal":"Launch a healthy lunch store opening campaign",
		"selected_entries":["documents"]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/start", body)
	req = req.WithContext(context.WithValue(req.Context(), currentUserKey{}, &auth.Claims{
		UserID: "user-from-token",
		Email:  "user@example.com",
		Role:   "user",
	}))
	rec := httptest.NewRecorder()

	handler.CreateAndStart(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if creator.params.UserID != "user-from-token" {
		t.Fatalf("expected user id from auth context, got %q", creator.params.UserID)
	}
}

func TestListProjectsReturnsCurrentUserProjects(t *testing.T) {
	creator := &fakeProjectCreator{
		projects: []repository.Project{{
			ID:        "project-1",
			UserID:    "user-from-token",
			Name:      "Fresh Bowl",
			Type:      "store_opening",
			Goal:      "Launch campaign",
			Status:    "running",
			CreatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		}},
		total: 1,
	}
	handler := NewProjectHandler(&fakeProjectStarter{}, creator)
	req := requestWithCurrentUser(http.MethodGet, "/api/v1/projects?page=1&page_size=20", nil)
	rec := httptest.NewRecorder()

	handler.ListProjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if creator.listParams.UserID != "user-from-token" {
		t.Fatalf("expected user id from token, got %q", creator.listParams.UserID)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "project-1" || body.Pagination.Total != 1 {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestGetProjectReturnsDetailForCurrentUser(t *testing.T) {
	creator := &fakeProjectCreator{
		projectDetail: &repository.ProjectDetail{
			Project: repository.Project{
				ID:        "project-1",
				UserID:    "user-from-token",
				Name:      "Fresh Bowl",
				Type:      "store_opening",
				Goal:      "Launch campaign",
				Status:    "running",
				CreatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
			},
			Tasks: []repository.Task{{
				ID:        "task-1",
				SkillType: "research",
				Title:     "Research Skill",
				Status:    "done",
			}},
		},
	}
	handler := NewProjectHandler(&fakeProjectStarter{}, creator)
	req := requestWithCurrentUser(http.MethodGet, "/api/v1/projects/project-1", nil)
	req.SetPathValue("id", "project-1")
	rec := httptest.NewRecorder()

	handler.GetProject(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if creator.detailProjectID != "project-1" || creator.detailUserID != "user-from-token" {
		t.Fatalf("unexpected detail args: project=%q user=%q", creator.detailProjectID, creator.detailUserID)
	}
	var body struct {
		Data struct {
			ID    string `json:"id"`
			Tasks []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ID != "project-1" || len(body.Data.Tasks) != 1 {
		t.Fatalf("unexpected project detail: %#v", body.Data)
	}
}

func TestGetDeliverableReturnsContentForCurrentUser(t *testing.T) {
	creator := &fakeProjectCreator{
		deliverable: &repository.Deliverable{
			ID:        "deliverable-1",
			ProjectID: "project-1",
			Entry:     "documents",
			Type:      "research",
			Title:     "Research Skill 输出",
			Format:    "markdown",
			Content:   "research content",
			Version:   1,
			Status:    "done",
			CreatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		},
	}
	handler := NewProjectHandler(&fakeProjectStarter{}, creator)
	req := requestWithCurrentUser(http.MethodGet, "/api/v1/deliverables/deliverable-1", nil)
	req.SetPathValue("id", "deliverable-1")
	rec := httptest.NewRecorder()

	handler.GetDeliverable(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if creator.deliverableID != "deliverable-1" || creator.deliverableUserID != "user-from-token" {
		t.Fatalf("unexpected deliverable args: id=%q user=%q", creator.deliverableID, creator.deliverableUserID)
	}
	var body struct {
		Data struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ID != "deliverable-1" || body.Data.Content != "research content" {
		t.Fatalf("unexpected deliverable response: %#v", body.Data)
	}
}

func TestListDeliverablesReturnsProjectDeliverablesForCurrentUser(t *testing.T) {
	creator := &fakeProjectCreator{
		deliverables: []repository.Deliverable{{
			ID:        "deliverable-1",
			ProjectID: "project-1",
			Entry:     "documents",
			Type:      "research",
			Title:     "Research Skill 输出",
			Format:    "markdown",
			Content:   "research content",
			Version:   1,
			Status:    "done",
			CreatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		}},
	}
	handler := NewProjectHandler(&fakeProjectStarter{}, creator)
	req := requestWithCurrentUser(http.MethodGet, "/api/v1/projects/project-1/deliverables?entry=documents", nil)
	req.SetPathValue("id", "project-1")
	rec := httptest.NewRecorder()

	handler.ListDeliverables(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if creator.listDeliverablesProjectID != "project-1" || creator.listDeliverablesUserID != "user-from-token" || creator.listDeliverablesEntry != "documents" {
		t.Fatalf("unexpected list deliverables args: project=%q user=%q entry=%q", creator.listDeliverablesProjectID, creator.listDeliverablesUserID, creator.listDeliverablesEntry)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != "deliverable-1" {
		t.Fatalf("unexpected deliverables response: %#v", body.Data)
	}
}

type fakeProjectCreator struct {
	projectID                 string
	params                    repository.CreateProjectParams
	projects                  []repository.Project
	total                     int
	listParams                repository.ListProjectsParams
	projectDetail             *repository.ProjectDetail
	detailProjectID           string
	detailUserID              string
	deliverable               *repository.Deliverable
	deliverableID             string
	deliverableUserID         string
	deliverables              []repository.Deliverable
	listDeliverablesProjectID string
	listDeliverablesUserID    string
	listDeliverablesEntry     string
}

func (c *fakeProjectCreator) CreateProject(_ context.Context, params repository.CreateProjectParams) (string, error) {
	c.params = params
	return c.projectID, nil
}

func (c *fakeProjectCreator) ListProjects(_ context.Context, params repository.ListProjectsParams) ([]repository.Project, int, error) {
	c.listParams = params
	return c.projects, c.total, nil
}

func (c *fakeProjectCreator) GetProjectDetail(_ context.Context, projectID string, userID string) (*repository.ProjectDetail, error) {
	c.detailProjectID = projectID
	c.detailUserID = userID
	return c.projectDetail, nil
}

func (c *fakeProjectCreator) GetDeliverable(_ context.Context, deliverableID string, userID string) (*repository.Deliverable, error) {
	c.deliverableID = deliverableID
	c.deliverableUserID = userID
	return c.deliverable, nil
}

func (c *fakeProjectCreator) ListDeliverables(_ context.Context, projectID string, userID string, entry string) ([]repository.Deliverable, error) {
	c.listDeliverablesProjectID = projectID
	c.listDeliverablesUserID = userID
	c.listDeliverablesEntry = entry
	return c.deliverables, nil
}

type fakeProjectStarter struct {
	projectID string
	brief     *skill.ProjectBrief
	entries   []string
}

func (s *fakeProjectStarter) StartAsync(_ context.Context, projectID string, brief *skill.ProjectBrief, selectedEntries []string) {
	s.projectID = projectID
	s.brief = brief
	s.entries = selectedEntries
}

func (s *fakeProjectStarter) GetStatus(string) (*orchestrator.ProjectStatus, bool) {
	return nil, false
}

func requestWithCurrentUser(method string, target string, body *bytes.Buffer) *http.Request {
	var reader *bytes.Buffer
	if body == nil {
		reader = bytes.NewBuffer(nil)
	} else {
		reader = body
	}
	req := httptest.NewRequest(method, target, reader)
	return req.WithContext(context.WithValue(req.Context(), currentUserKey{}, &auth.Claims{
		UserID: "user-from-token",
		Email:  "user@example.com",
		Role:   "user",
	}))
}
