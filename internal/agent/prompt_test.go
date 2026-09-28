package agent

import (
	"strings"
	"testing"
)

func TestRenderSystemPrompt(t *testing.T) {
	promptPath := "../../prompts/system.md"
	rendered, err := RenderSystemPrompt(promptPath, "Gita", "Glowria Aesthetic Clinic")
	if err != nil {
		t.Fatalf("failed to render prompt: %v", err)
	}

	if strings.Contains(rendered, "{{aiName}}") {
		t.Errorf("unrendered {{aiName}} remains in prompt")
	}
	if !strings.Contains(rendered, "Gita") {
		t.Errorf("expected Gita in rendered prompt")
	}
	if !strings.Contains(rendered, "Glowria Aesthetic Clinic") {
		t.Errorf("expected Glowria Aesthetic Clinic in rendered prompt")
	}
	if strings.Contains(rendered, "{{companyName}}") {
		t.Errorf("unrendered {{companyName}} remains in prompt")
	}
	if strings.Contains(rendered, "{{currentDateContext}}") {
		t.Errorf("unrendered {{currentDateContext}} remains in prompt")
	}
	if strings.Contains(rendered, "{{bookingContext}}") {
		t.Errorf("unrendered {{bookingContext}} remains in prompt")
	}
	if strings.Contains(rendered, "{{leadContext}}") {
		t.Errorf("unrendered {{leadContext}} remains in prompt")
	}
	if strings.Contains(rendered, "{{conversationSummary}}") {
		t.Errorf("unrendered {{conversationSummary}} remains in prompt")
	}
	if strings.Contains(rendered, "{{knowledgeBase}}") {
		t.Errorf("unrendered {{knowledgeBase}} remains in prompt")
	}
	if strings.Contains(rendered, "{{additionalContext}}") {
		t.Errorf("unrendered {{additionalContext}} remains in prompt")
	}
	// Assert no unrendered {{ }} placeholders remain in the prompt
	if strings.Contains(rendered, "{{") {
		t.Errorf("unexpected unrendered placeholder remains in rendered prompt")
	}
}

func TestRenderSystemPrompt_FileNotFound(t *testing.T) {
	_, err := RenderSystemPrompt("invalid-path-to-prompt.md", "Gita", "Glowria")
	if err == nil {
		t.Fatalf("expected error for non-existent file, got nil")
	}
}
