package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

type fakeProjectCreator struct {
	projectID string
	params    repository.CreateProjectParams
}

func (c *fakeProjectCreator) CreateProject(_ context.Context, params repository.CreateProjectParams) (string, error) {
	c.params = params
	return c.projectID, nil
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
