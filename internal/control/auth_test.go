package control

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAdminAuthenticationLifecycle(t *testing.T) {
	auth := newAdminAuth("admin-token-at-least-16")
	protected := auth.require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	bad := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"wrong"}`))
	bad.RemoteAddr = "127.0.0.1:1234"
	response = httptest.NewRecorder()
	auth.login().ServeHTTP(response, bad)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", response.Code)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"admin-token-at-least-16"}`))
	login.RemoteAddr = "127.0.0.1:1234"
	response = httptest.NewRecorder()
	auth.login().ServeHTTP(response, login)
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("login cookie = %+v", cookies)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	request.AddCookie(cookies[0])
	response = httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authenticated status = %d", response.Code)
	}

	logout := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	response = httptest.NewRecorder()
	auth.logout().ServeHTTP(response, logout)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout response status=%d cookies=%+v", response.Code, response.Result().Cookies())
	}
}

func TestAdminSessionRejectsExpiredOrTamperedCookie(t *testing.T) {
	auth := newAdminAuth("admin-token-at-least-16")
	for _, value := range []string{
		func() string {
			expires := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
			return expires + "." + auth.sign(expires)
		}(),
		strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + ".tampered",
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: value})
		if auth.authenticated(request) {
			t.Fatalf("invalid cookie was authenticated: %q", value)
		}
	}
}

func TestAdminAuthenticationCanBeDisabled(t *testing.T) {
	auth := newAdminAuth("")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	response := httptest.NewRecorder()
	auth.sessionStatus().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authenticated":true`) || !strings.Contains(response.Body.String(), `"required":false`) {
		t.Fatalf("session status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminLoginRateLimit(t *testing.T) {
	auth := newAdminAuth("admin-token-at-least-16")
	for attempt := 0; attempt < 5; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"wrong"}`))
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		auth.login().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status=%d", attempt, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session", strings.NewReader(`{"token":"admin-token-at-least-16"}`))
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	auth.login().ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
		t.Fatalf("rate limit status=%d retry=%q", response.Code, response.Header().Get("Retry-After"))
	}
}
