package control

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const adminSessionCookie = "dba_admin_session"

const maxLoginFailureClients = 1024

type adminAuth struct {
	token     []byte
	tokenHash [sha256.Size]byte
	mu        sync.Mutex
	failures  map[string]loginFailures
}

type loginFailures struct {
	count int
	since time.Time
}

func newAdminAuth(token string) *adminAuth {
	return &adminAuth{token: []byte(token), tokenHash: sha256.Sum256([]byte(token)), failures: make(map[string]loginFailures)}
}

func (a *adminAuth) required() bool { return len(a.token) > 0 }

func (a *adminAuth) authenticated(r *http.Request) bool {
	if !a.required() {
		return true
	}
	cookie, err := r.Cookie(adminSessionCookie)
	if err != nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	expiresUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() >= expiresUnix {
		return false
	}
	expected := a.sign(parts[0])
	return len(parts[1]) == len(expected) && subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) == 1
}

func (a *adminAuth) sign(expires string) string {
	mac := hmac.New(sha256.New, a.token)
	_, _ = mac.Write([]byte("database-accelerator-admin-session-v1:" + expires))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *adminAuth) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *adminAuth) sessionStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{
			"required":      a.required(),
			"authenticated": a.authenticated(r),
		})
	})
}

func (a *adminAuth) login() http.Handler {
	type requestBody struct {
		Token string `json:"token"`
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.required() {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
			return
		}
		client := remoteHost(r.RemoteAddr)
		if a.rateLimited(client, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many login attempts"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var body requestBody
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid login request"})
			return
		}
		providedHash := sha256.Sum256([]byte(body.Token))
		valid := subtle.ConstantTimeCompare(providedHash[:], a.tokenHash[:]) == 1
		if !valid {
			a.recordFailure(client, time.Now())
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid admin token"})
			return
		}
		a.clearFailures(client)
		expiresAt := time.Now().Add(8 * time.Hour)
		expires := strconv.FormatInt(expiresAt.Unix(), 10)
		http.SetCookie(w, &http.Cookie{
			Name:     adminSessionCookie,
			Value:    expires + "." + a.sign(expires),
			Path:     "/",
			MaxAge:   8 * 60 * 60,
			Expires:  expiresAt,
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
	})
}

func (a *adminAuth) logout() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     adminSessionCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteStrictMode,
		})
		writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
	})
}

func (a *adminAuth) rateLimited(client string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	client = a.failureKeyLocked(client, now)
	entry := a.failures[client]
	if now.Sub(entry.since) >= time.Minute {
		delete(a.failures, client)
		return false
	}
	return entry.count >= 5
}

func (a *adminAuth) recordFailure(client string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	client = a.failureKeyLocked(client, now)
	entry := a.failures[client]
	if entry.since.IsZero() || now.Sub(entry.since) >= time.Minute {
		entry = loginFailures{since: now}
	}
	entry.count++
	a.failures[client] = entry
}

func (a *adminAuth) failureKeyLocked(client string, now time.Time) string {
	if _, exists := a.failures[client]; exists || len(a.failures) < maxLoginFailureClients-1 {
		return client
	}
	for key, entry := range a.failures {
		if now.Sub(entry.since) >= time.Minute {
			delete(a.failures, key)
		}
	}
	if len(a.failures) >= maxLoginFailureClients {
		return "overflow"
	}
	return client
}

func (a *adminAuth) clearFailures(client string) {
	a.mu.Lock()
	delete(a.failures, client)
	a.mu.Unlock()
}

func remoteHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}
