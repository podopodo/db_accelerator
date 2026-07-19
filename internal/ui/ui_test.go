package ui

import (
	"bytes"
	"io/fs"
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
		{path: "/assets/brand/logo-mark.svg", contentType: "image/svg+xml", contains: "Database Accelerator mark"},
		{path: "/assets/fonts/BricolageGrotesque-Variable.ttf", contentType: "font/ttf"},
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
			if test.contains != "" && !strings.Contains(response.Body.String(), test.contains) {
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

func TestDashboardQualityContractAssets(t *testing.T) {
	index := readEmbedded(t, "assets/index.html")
	css := readEmbedded(t, "assets/app.css")
	script := readEmbedded(t, "assets/app.js")
	combined := strings.ToLower(string(index) + string(css) + string(script))
	if !strings.Contains(string(index), "/assets/app.css?v=5") || !strings.Contains(string(index), "/assets/app.js?v=5") {
		t.Fatal("dashboard assets are missing their binary revision key")
	}
	for _, marker := range []string{"data-view=\"performance\"", "benchmark-reduction", "benchmark-caveat"} {
		if !strings.Contains(string(index), marker) {
			t.Fatalf("dashboard performance evidence missing %q", marker)
		}
	}

	for _, forbidden := range []string{
		"https://", "http://", "glassmorphism", "gradient text", "revolutionize", "world-class", "seamlessly",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("dashboard contains forbidden or network-dependent pattern %q", forbidden)
		}
	}
	for _, required := range []string{
		"@font-face", "--hue-1-500", "--hue-2-500", "--hue-3-500",
		"prefers-reduced-motion", "forced-colors", "@media (max-width: 900px)", "@media (max-width: 640px)",
	} {
		if !strings.Contains(string(css), required) {
			t.Fatalf("dashboard CSS missing quality contract marker %q", required)
		}
	}
	for _, name := range []string{
		"assets/fonts/BricolageGrotesque-Variable.ttf",
		"assets/fonts/Lora-Variable.ttf",
		"assets/fonts/IBMPlexMono-Regular.ttf",
		"assets/fonts/IBMPlexMono-SemiBold.ttf",
	} {
		font := readEmbedded(t, name)
		if len(font) < 4 || !bytes.Equal(font[:4], []byte{0x00, 0x01, 0x00, 0x00}) {
			t.Fatalf("%s is not an embedded TrueType font", name)
		}
	}
}

func TestDashboardRejectsUnlistedAssets(t *testing.T) {
	for _, target := range []string{"/assets/fonts/OFL.txt", "/assets/fonts/missing.ttf"} {
		response := httptest.NewRecorder()
		Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", target, response.Code)
		}
	}
}

func readEmbedded(t *testing.T, name string) []byte {
	t.Helper()
	data, err := fs.ReadFile(embedded, name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
