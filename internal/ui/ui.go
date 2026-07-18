// Package ui embeds the operations cockpit into the Go binary.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
)

//go:embed assets/*
var embedded embed.FS

func Handler() http.Handler {
	content, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
		name := path.Base(r.URL.Path)
		var contentType string
		switch name {
		case "app.css":
			contentType = "text/css; charset=utf-8"
		case "app.js":
			contentType = "application/javascript; charset=utf-8"
		default:
			http.NotFound(w, r)
			return
		}
		data, readErr := fs.ReadFile(content, name)
		if readErr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, readErr := fs.ReadFile(content, "index.html")
		if readErr != nil {
			http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	return mux
}
