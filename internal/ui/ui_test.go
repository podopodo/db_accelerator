package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedDashboardAndAssets(t *testing.T) {
	handler := Handler()
	for _, test := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/", contentType: "text/html", contains: "Connection Pressure Map"},
		{path: "/assets/app.css", contentType: "text/css", contains: "--hue-1-500"},
		{path: "/assets/app.js", contentType: "application/javascript", contains: "/api/v1/status"},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d", response.Code)
			}
			if !strings.Contains(response.Header().Get("Content-Type"), test.contentType) {
				t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
			}
			if !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("body missing %q", test.contains)
			}
		})
	}
}

func TestUnknownDashboardRouteIs404(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d", response.Code)
	}
}
