package repository

import (
	"testing"

	"github.com/mosaic-app/mosaic/backend/internal/skill"
)

func TestDeliverableFromOutputMapsSkillOutput(t *testing.T) {
	output := &skill.Output{
		SkillType: skill.TypeResearch,
		EntryType: "documents",
		Format:    skill.FormatMarkdown,
		Content:   "research content",
	}

	deliverable := deliverableFromOutput(output)

	if deliverable.Entry != "documents" {
		t.Fatalf("expected documents entry, got %q", deliverable.Entry)
	}
	if deliverable.Type != "research" {
		t.Fatalf("expected research type, got %q", deliverable.Type)
	}
	if deliverable.Title != "Research Skill 输出" {
		t.Fatalf("unexpected title: %q", deliverable.Title)
	}
	if deliverable.Format != "markdown" {
		t.Fatalf("expected markdown format, got %q", deliverable.Format)
	}
	if deliverable.Content != "research content" {
		t.Fatalf("unexpected content: %q", deliverable.Content)
	}
	if deliverable.Status != "done" {
		t.Fatalf("expected done status, got %q", deliverable.Status)
	}
}

func TestTaskTitleUsesSkillType(t *testing.T) {
	if got := taskTitle(skill.TypeDeck); got != "Deck Skill" {
		t.Fatalf("unexpected task title: %q", got)
	}
}
