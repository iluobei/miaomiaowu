package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchPeerCertSha256(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	hostPort := strings.TrimPrefix(server.URL, "https://")
	host, portText, _ := strings.Cut(hostPort, ":")
	var port int
	_, _ = fmt.Sscanf(portText, "%d", &port)
	got, err := fetchPeerCertSha256(t.Context(), host, port, host, "")
	if err != nil {
		t.Fatal(err)
	}
	cert := server.TLS.Certificates[0].Certificate[0]
	wantBytes := sha256.Sum256(cert)
	if want := hex.EncodeToString(wantBytes[:]); got != want {
		t.Fatalf("fingerprint=%s, want %s", got, want)
	}
}
