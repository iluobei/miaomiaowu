package handler

import "testing"

func TestDisableSkipCertVerifyInJSON(t *testing.T) {
	raw := `{"name":"n","skip-cert-verify":true}`
	if !disableSkipCertVerifyInJSON(&raw) || raw == "" {
		t.Fatal("true skip-cert-verify was not disabled")
	}
	if disableSkipCertVerifyInJSON(&raw) {
		t.Fatal("already-disabled config should be skipped")
	}
	without := `{"name":"n"}`
	if disableSkipCertVerifyInJSON(&without) {
		t.Fatal("config without skip-cert-verify should be skipped")
	}
}
