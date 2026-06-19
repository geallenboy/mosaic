package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/mosaic-app/mosaic/backend/internal/skill"
)

func TestExecuteSavesDeliverableForSuccessfulSkill(t *testing.T) {
	store := newFakeStore()
	registry := skill.NewRegistry()
	registry.Register(&testSkill{
		skillType: skill.TypeResearch,
		entries:   []string{"documents"},
		output: &skill.Output{
			SkillType: skill.TypeResearch,
			EntryType: "documents",
			Format:    skill.FormatMarkdown,
			Content:   "market research",
		},
	})

	orch := NewWithStore(registry, store)
	projectID := "project-1"
	orch.startInMemory(projectID)

	if err := orch.execute(context.Background(), projectID, testBrief(projectID), []string{"documents"}); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if len(store.deliverables) != 1 {
		t.Fatalf("expected 1 deliverable, got %d", len(store.deliverables))
	}
	if store.deliverables[0].Content != "market research" {
		t.Fatalf("unexpected deliverable content: %q", store.deliverables[0].Content)
	}
}

func TestExecutePersistsFailedTaskStatus(t *testing.T) {
	store := newFakeStore()
	registry := skill.NewRegistry()
	registry.Register(&testSkill{
		skillType: skill.TypeResearch,
		entries:   []string{"documents"},
		err:       errors.New("llm unavailable"),
	})

	orch := NewWithStore(registry, store)
	projectID := "project-2"
	orch.startInMemory(projectID)

	if err := orch.execute(context.Background(), projectID, testBrief(projectID), []string{"documents"}); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	task, ok := store.taskStatuses[skill.TypeResearch]
	if !ok {
		t.Fatal("expected research task status to be persisted")
	}
	if task.Status != "failed" {
		t.Fatalf("expected failed task status, got %q", task.Status)
	}
	if task.ErrorMsg != "llm unavailable" {
		t.Fatalf("unexpected error message: %q", task.ErrorMsg)
	}
}

func TestGetStatusRestoresFromStoreWhenMemoryIsMissing(t *testing.T) {
	store := newFakeStore()
	projectID := "project-3"
	store.restoredStatus = &ProjectStatus{
		ProjectID: projectID,
		Phase:     PhaseDone,
		Progress:  100,
		Tasks: []TaskStatus{{
			SkillType: skill.TypeResearch,
			Status:    "done",
		}},
	}

	orch := NewWithStore(skill.NewRegistry(), store)

	status, found := orch.GetStatus(projectID)
	if !found {
		t.Fatal("expected status to be restored from store")
	}
	if status.Phase != PhaseDone || status.Progress != 100 {
		t.Fatalf("unexpected restored status: phase=%s progress=%d", status.Phase, status.Progress)
	}
}

type testSkill struct {
	skillType skill.Type
	entries   []string
	deps      []skill.Type
	output    *skill.Output
	err       error
}

func (s *testSkill) Type() skill.Type { return s.skillType }

func (s *testSkill) Execute(context.Context, skill.Input) (*skill.Output, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.output, nil
}

func (s *testSkill) SupportedEntries() []string { return s.entries }

func (s *testSkill) Dependencies() []skill.Type { return s.deps }

func testBrief(projectID string) *skill.ProjectBrief {
	return &skill.ProjectBrief{
		ProjectID:        projectID,
		Objective:        "Launch a store opening campaign",
		Audience:         "nearby office workers",
		Positioning:      "Fresh lunch brand",
		ValueProposition: "Healthy meals nearby",
		ToneOfVoice:      "warm",
		VisualDirection:  "clean",
		Industry:         "restaurant",
		Market:           "Shenzhen",
	}
}

type fakeStore struct {
	deliverables   []*skill.Output
	taskStatuses   map[skill.Type]TaskStatus
	restoredStatus *ProjectStatus
}

func newFakeStore() *fakeStore {
	return &fakeStore{taskStatuses: make(map[skill.Type]TaskStatus)}
}

func (s *fakeStore) InitTasks(context.Context, string, []TaskStatus) error {
	return nil
}

func (s *fakeStore) UpdateTaskStatus(_ context.Context, _ string, task TaskStatus) error {
	s.taskStatuses[task.SkillType] = task
	return nil
}

func (s *fakeStore) SaveDeliverable(_ context.Context, _ string, output *skill.Output) error {
	s.deliverables = append(s.deliverables, output)
	return nil
}

func (s *fakeStore) UpdateProjectStatus(context.Context, string, Phase, int, string) error {
	return nil
}

func (s *fakeStore) GetStatus(context.Context, string) (*ProjectStatus, error) {
	if s.restoredStatus == nil {
		return nil, ErrStatusNotFound
	}
	status := *s.restoredStatus
	return &status, nil
}

func (o *Orchestrator) startInMemory(projectID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.statuses[projectID] = &projectState{
		status: ProjectStatus{
			ProjectID: projectID,
			Phase:     PhaseBriefGeneration,
		},
		outputs: make(map[skill.Type]*skill.Output),
	}
}
