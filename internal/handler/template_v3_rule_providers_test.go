package handler

import (
	"strings"
	"testing"
)

func TestAppendRuleProvidersToTemplate(t *testing.T) {
	providers := map[string]any{
		"private": map[string]any{
			"type":     "http",
			"behavior": "domain",
			"url":      "https://example.com/private.yaml",
		},
	}

	got, err := appendRuleProvidersToTemplate("mode: rule\nrules:\n  - RULE-SET,private,DIRECT", providers)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rule-providers:", "private:", "https://example.com/private.yaml"} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated template missing %q:\n%s", want, got)
		}
	}
}

func TestAppendRuleProvidersDoesNotDuplicateExistingSection(t *testing.T) {
	content := "mode: rule\nrule-providers:\n  existing:\n    type: file\n"
	got, err := appendRuleProvidersToTemplate(content, map[string]any{
		"new": map[string]any{"type": "http"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Fatalf("existing rule-providers should be preserved unchanged:\n%s", got)
	}
}
