package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed dist/*
var embeddedFiles embed.FS

var (
	initOnce    sync.Once
	staticFS    fs.FS
	staticFiles http.Handler
	indexBytes  []byte
	indexMod    time.Time
)

func initialize() {
	sub, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		panic(err)
	}

	staticFS = sub
	staticFiles = http.FileServer(http.FS(sub))

	indexBytes, err = fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err)
	}

	if info, err := fs.Stat(sub, "index.html"); err == nil {
		indexMod = info.ModTime()
	} else {
		indexMod = time.Now()
	}
}

// Handler returns an HTTP handler that serves the embedded frontend SPA.
func Handler() http.Handler {
	initOnce.Do(initialize)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/traffic/") {
			http.NotFound(w, r)
			return
		}

		cleaned := path.Clean(r.URL.Path)
		if cleaned == "." {
			cleaned = "/"
		}

		if cleaned == "/" {
			serveIndex(w, r)
			return
		}

		resource := strings.TrimPrefix(cleaned, "/")
		if resource == "" {
			serveIndex(w, r)
			return
		}

		if fileExists(resource) {
			staticFiles.ServeHTTP(w, r)
			return
		}

		serveIndex(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(initialize)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	reader := bytes.NewReader(indexBytes)
	http.ServeContent(w, r, "index.html", indexMod, reader)
}

func fileExists(name string) bool {
	initOnce.Do(initialize)

	info, err := fs.Stat(staticFS, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
