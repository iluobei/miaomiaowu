package handler

import "testing"

func TestSanitizeSubscribeFilename(t *testing.T) {
	for _, invalid := range []string{"", "../a.yaml", "dir/a.yaml", `dir\a.yaml`} {
		if _, err := sanitizeSubscribeFilename(invalid); err == nil {
			t.Errorf("unsafe filename %q accepted", invalid)
		}
	}
	if got, err := sanitizeSubscribeFilename("safe"); err != nil || got != "safe.yaml" {
		t.Fatalf("sanitize safe filename = %q, %v", got, err)
	}
}
