package handler

import (
	"strings"
	"testing"
)

func TestInjectProxiesIntoSurgeTemplate(t *testing.T) {
	template := "[General]\nloglevel = notify\n\n[Proxy]\nold = direct\n# keep me\n\n[Proxy Group]\nSelect = select, DIRECT\n\n[Rule]\nFINAL,Select"
	result, err := injectProxiesIntoSurgeTemplate(template, []map[string]any{{
		"name": "node-a", "type": "ss", "server": "example.com", "port": 443,
		"cipher": "aes-128-gcm", "password": "secret",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"node-a=ss,example.com,443", "# keep me", "[Proxy Group]", "FINAL,Select"} {
		if !strings.Contains(result, want) {
			t.Errorf("Surge result missing %q:\n%s", want, result)
		}
	}
	if strings.Contains(result, "old = direct") {
		t.Fatalf("old proxy line was not replaced:\n%s", result)
	}
}

func TestLooksLikeSurgeTemplate(t *testing.T) {
	if !looksLikeSurgeTemplate("[General]\nloglevel = notify") {
		t.Fatal("Surge template was not detected")
	}
	if looksLikeSurgeTemplate("proxies: []\nrules: []") {
		t.Fatal("Clash YAML was detected as Surge")
	}
}
