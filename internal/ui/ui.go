// Package ui embeds the operations cockpit into the Go binary.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
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
		name := strings.TrimPrefix(r.URL.Path, "/assets/")
		var contentType string
		switch name {
		case "app.css":
			contentType = "text/css; charset=utf-8"
		case "app.js":
			contentType = "application/javascript; charset=utf-8"
		case "brand/logo-mark.svg", "brand/logo-lockup.svg":
			contentType = "image/svg+xml; charset=utf-8"
		case "fonts/BricolageGrotesque-Variable.ttf", "fonts/Lora-Variable.ttf", "fonts/IBMPlexMono-Regular.ttf", "fonts/IBMPlexMono-SemiBold.ttf":
			contentType = "font/ttf"
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
