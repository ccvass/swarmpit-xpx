package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ccvass/swarmpit-xpx/internal/store"
)

func setupAuth(t *testing.T) {
	t.Helper()
	if err := store.Init(t.TempDir()); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
}

func TestGenerateAndValidateJWT(t *testing.T) {
	setupAuth(t)
	token, err := GenerateJWT("alice", "admin")
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("ValidateJWT: %v", err)
	}
	if claims.Usr.Username != "alice" || claims.Usr.Role != "admin" {
		t.Errorf("claims: %+v", claims.Usr)
	}
}

func TestValidateJWT_Bearer(t *testing.T) {
	setupAuth(t)
	token, _ := GenerateJWT("bob", "user")
	claims, err := ValidateJWT("Bearer " + token)
	if err != nil {
		t.Fatalf("ValidateJWT with Bearer: %v", err)
	}
	if claims.Usr.Username != "bob" {
		t.Error("username mismatch")
	}
}

func TestValidateJWT_Invalid(t *testing.T) {
	setupAuth(t)
	_, err := ValidateJWT("garbage.token.here")
	if err == nil {
		t.Error("should reject invalid token")
	}
}

func TestDecodeBasic(t *testing.T) {
	// "alice:secret" base64 = "YWxpY2U6c2VjcmV0"
	u, p, ok := DecodeBasic("Basic YWxpY2U6c2VjcmV0")
	if !ok || u != "alice" || p != "secret" {
		t.Errorf("got %q %q %v", u, p, ok)
	}
	_, _, ok = DecodeBasic("invalid")
	if ok {
		t.Error("should fail on invalid base64")
	}
	_, _, ok = DecodeBasic("Basic " + "bm9jb2xvbg==") // "nocolon"
	if ok {
		t.Error("should fail without colon")
	}
}

func TestMiddleware_NoToken(t *testing.T) {
	setupAuth(t)
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	setupAuth(t)
	token, _ := GenerateJWT("alice", "admin")
	var gotUser, gotRole string
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.Header.Get("X-User")
		gotRole = r.Header.Get("X-Role")
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotUser != "alice" || gotRole != "admin" {
		t.Errorf("got user=%q role=%q", gotUser, gotRole)
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	setupAuth(t)
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAdminOnly(t *testing.T) {
	handler := AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	// Non-admin
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Role", "user")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
	// Admin
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Role", "admin")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Errorf("expected 200, got %d", rec2.Code)
	}
}

func TestWriteOnly(t *testing.T) {
	handler := WriteOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	// Viewer blocked
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-Role", "viewer")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for viewer, got %d", rec.Code)
	}
	// User allowed
	req2 := httptest.NewRequest("POST", "/", nil)
	req2.Header.Set("X-Role", "user")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Errorf("expected 200 for user, got %d", rec2.Code)
	}
	// Admin allowed
	req3 := httptest.NewRequest("POST", "/", nil)
	req3.Header.Set("X-Role", "admin")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != 200 {
		t.Errorf("expected 200 for admin, got %d", rec3.Code)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
