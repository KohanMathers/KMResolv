package dashboard

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/server"
)

func TestSignVerifyToken(t *testing.T) {
	signed := signToken(sessionToken)
	if !verifyToken(signed) {
		t.Error("freshly signed token should verify successfully")
	}
}

func TestVerifyTokenTamperedHMAC(t *testing.T) {
	signed := signToken(sessionToken)
	parts := strings.SplitN(signed, ".", 2)
	tampered := parts[0] + ".deadbeef00112233"
	if verifyToken(tampered) {
		t.Error("token with tampered HMAC should not verify")
	}
}

func TestVerifyTokenWrongFormat(t *testing.T) {
	if verifyToken("") {
		t.Error("empty string should not verify")
	}
	if verifyToken("nodeliimiter") {
		t.Error("token without '.' separator should not verify")
	}
}

func TestVerifyTokenWrongSessionValue(t *testing.T) {
	mac := hmac.New(sha256.New, sessionSecret)
	mac.Write([]byte("other-token-value"))
	forged := "other-token-value." + hex.EncodeToString(mac.Sum(nil))
	if verifyToken(forged) {
		t.Error("token not matching sessionToken should not verify")
	}
}

func TestNonceIssueConsume(t *testing.T) {
	ns := &nonceStore{nonces: make(map[string]time.Time)}

	n := ns.Issue()
	if n == "" {
		t.Fatal("issued nonce should not be empty")
	}
	if len(n) != 64 {
		t.Errorf("nonce length = %d, want 64 hex chars", len(n))
	}
	if !ns.Consume(n) {
		t.Error("first Consume should return true")
	}
	if ns.Consume(n) {
		t.Error("second Consume should return false (one-time use)")
	}
}

func TestNonceConsumeUnknown(t *testing.T) {
	ns := &nonceStore{nonces: make(map[string]time.Time)}

	if ns.Consume("nonexistent-nonce") {
		t.Error("unknown nonce should return false")
	}
}

func TestNonceConsumeExpired(t *testing.T) {
	ns := &nonceStore{nonces: make(map[string]time.Time)}

	ns.mu.Lock()
	ns.nonces["stale"] = time.Now().Add(-1 * time.Second)
	ns.mu.Unlock()

	if ns.Consume("stale") {
		t.Error("expired nonce should return false")
	}
	ns.mu.Lock()
	_, ok := ns.nonces["stale"]
	ns.mu.Unlock()
	if ok {
		t.Error("expired nonce should be removed from the store")
	}
}

func TestNonceIssueUnique(t *testing.T) {
	ns := &nonceStore{nonces: make(map[string]time.Time)}

	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		n := ns.Issue()
		if seen[n] {
			t.Errorf("duplicate nonce issued: %s", n)
		}
		seen[n] = true
	}
}

func TestJsonOK(t *testing.T) {
	rec := httptest.NewRecorder()
	jsonOK(rec, map[string]any{"key": "value", "num": 42})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if out["key"] != "value" {
		t.Errorf("key = %v, want value", out["key"])
	}
}

func TestIsAuthenticatedNoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if isAuthenticated(req) {
		t.Error("request without cookie should not be authenticated")
	}
}

func TestIsAuthenticatedValidCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "kmresolv_session",
		Value: signToken(sessionToken),
	})
	if !isAuthenticated(req) {
		t.Error("request with valid session cookie should be authenticated")
	}
}

func TestIsAuthenticatedTamperedCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "kmresolv_session",
		Value: "tampered.0000000000000000",
	})
	if isAuthenticated(req) {
		t.Error("request with tampered cookie should not be authenticated")
	}
}

func TestIsAuthenticatedWrongCookieName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  "other_cookie",
		Value: signToken(sessionToken),
	})
	if isAuthenticated(req) {
		t.Error("wrong cookie name should not authenticate")
	}
}

func buildTestMux(cfg *config.Config, srv *server.Server) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/login/challenge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		jsonOK(w, map[string]any{"nonce": nonces.Issue()})
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Username string `json:"username"`
			Response string `json:"response"`
			Nonce    string `json:"nonce"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !nonces.Consume(body.Nonce) {
			jsonOK(w, map[string]any{"ok": false, "error": "invalid or expired challenge"})
			return
		}
		mac := hmac.New(sha256.New, []byte(cfg.Dashboard.Auth.Password))
		mac.Write([]byte(body.Nonce))
		expected := hex.EncodeToString(mac.Sum(nil))
		userOK := hmac.Equal([]byte(body.Username), []byte(cfg.Dashboard.Auth.Username))
		passOK := hmac.Equal([]byte(body.Response), []byte(expected))
		if !userOK || !passOK {
			jsonOK(w, map[string]any{"ok": false, "error": "invalid credentials"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "kmresolv_session",
			Value:    signToken(sessionToken),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   60 * 60 * 24 * 7,
		})
		jsonOK(w, map[string]any{"ok": true})
	})

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		if !requireAuth(w, r, cfg) {
			return
		}
		st := srv.Stats()
		jsonOK(w, map[string]any{"total_queries": st.TotalQueries})
	})

	return mux
}

func authCfg() *config.Config {
	return &config.Config{
		Dashboard: config.DashboardConfig{
			Enabled: true,
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "s3cr3t",
			},
		},
		Filtering: config.FilterConfig{Mode: "off"},
	}
}

func noAuthCfg() *config.Config {
	return &config.Config{
		Dashboard: config.DashboardConfig{Enabled: true},
		Filtering: config.FilterConfig{Mode: "off"},
	}
}

func TestChallengeEndpoint(t *testing.T) {
	cfg := authCfg()
	ts := httptest.NewServer(buildTestMux(cfg, server.New(cfg)))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/login/challenge", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatalf("POST challenge: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)

	nonce, ok := out["nonce"].(string)
	if !ok || nonce == "" {
		t.Error("expected non-empty nonce in response")
	}
	if len(nonce) != 64 {
		t.Errorf("nonce length = %d, want 64", len(nonce))
	}
}

func TestChallengeWrongMethod(t *testing.T) {
	cfg := authCfg()
	ts := httptest.NewServer(buildTestMux(cfg, server.New(cfg)))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/login/challenge")
	if err != nil {
		t.Fatalf("GET challenge: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET to POST-only endpoint should return 405, got %d", resp.StatusCode)
	}
}

func TestLoginSuccess(t *testing.T) {
	cfg := authCfg()
	ts := httptest.NewServer(buildTestMux(cfg, server.New(cfg)))
	defer ts.Close()

	nonce := getChallenge(t, ts.URL)

	mac := hmac.New(sha256.New, []byte("s3cr3t"))
	mac.Write([]byte(nonce))
	response := hex.EncodeToString(mac.Sum(nil))

	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"response": response,
		"nonce":    nonce,
	})
	loginResp, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	defer loginResp.Body.Close()

	var out map[string]any
	json.NewDecoder(loginResp.Body).Decode(&out)

	if ok, _ := out["ok"].(bool); !ok {
		t.Errorf("login should succeed, got: %v", out)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	cfg := authCfg()
	ts := httptest.NewServer(buildTestMux(cfg, server.New(cfg)))
	defer ts.Close()

	nonce := getChallenge(t, ts.URL)

	mac := hmac.New(sha256.New, []byte("wrongpassword"))
	mac.Write([]byte(nonce))
	response := hex.EncodeToString(mac.Sum(nil))

	body, _ := json.Marshal(map[string]string{
		"username": "admin",
		"response": response,
		"nonce":    nonce,
	})
	loginResp, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	defer loginResp.Body.Close()

	var out map[string]any
	json.NewDecoder(loginResp.Body).Decode(&out)

	if ok, _ := out["ok"].(bool); ok {
		t.Error("login with wrong password should fail")
	}
}

func TestLoginNonceReplay(t *testing.T) {
	cfg := authCfg()
	ts := httptest.NewServer(buildTestMux(cfg, server.New(cfg)))
	defer ts.Close()

	nonce := getChallenge(t, ts.URL)

	mac := hmac.New(sha256.New, []byte("s3cr3t"))
	mac.Write([]byte(nonce))
	response := hex.EncodeToString(mac.Sum(nil))

	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"response": response,
		"nonce":    nonce,
	})

	r1, _ := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(loginBody))
	r1.Body.Close()

	r2, err := http.Post(ts.URL+"/api/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("POST login replay: %v", err)
	}
	defer r2.Body.Close()

	var out map[string]any
	json.NewDecoder(r2.Body).Decode(&out)

	if ok, _ := out["ok"].(bool); ok {
		t.Error("nonce replay should be rejected on second use")
	}
}

func TestStatsNoAuthRequired(t *testing.T) {
	cfg := noAuthCfg()
	ts := httptest.NewServer(buildTestMux(cfg, server.New(cfg)))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatalf("GET stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("stats without auth config should return 200, got %d", resp.StatusCode)
	}
}

func getChallenge(t *testing.T, baseURL string) string {
	t.Helper()
	resp, err := http.Post(baseURL+"/api/login/challenge", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatalf("challenge request: %v", err)
	}
	defer resp.Body.Close()

	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)

	nonce, _ := out["nonce"].(string)
	if nonce == "" {
		t.Fatal("challenge response missing nonce")
	}
	return nonce
}
