package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizeCompletionErrorRemovesAPIKeyDetails(t *testing.T) {
	err := sanitizeCompletionError(errors.New("Incorrect API key provided: sk-test-secret"))

	if strings.Contains(err.Error(), "sk-test-secret") {
		t.Fatalf("expected sanitized error, got %q", err.Error())
	}
	if err.Error() != "openai complete: authentication failed" {
		t.Fatalf("unexpected sanitized error: %q", err.Error())
	}
}
