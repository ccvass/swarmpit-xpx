package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ccvass/swarmpit-xpx/internal/store"
)

func setupAPI(t *testing.T) {
	t.Helper()
	if err := store.Init(t.TempDir()); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	Version_ = "test-version"
}

// ── parseImage ──

func TestParseImage(t *testing.T) {
	tests := []struct {
		input      string
		name, tag  string
		hasDigest  bool
	}{
		{"nginx:latest", "nginx", "latest", false},
		{"nginx", "nginx", "", false},
		{"registry.io/repo:v1.0@sha256:abc", "registry.io/repo", "v1.0", true},
		{"ghcr.io/org/img:tag", "ghcr.io/org/img", "tag", false},
		{"myrepo:8080/image:v2", "myrepo:8080/image", "v2", false},
	}
	for _, tt := range tests {
		name, tag, digest := parseImage(tt.input)
		if name != tt.name {
			t.Errorf("parseImage(%q) name=%q want %q", tt.input, name, tt.name)
		}
		if tag != tt.tag {
			t.Errorf("parseImage(%q) tag=%q want %q", tt.input, tag, tt.tag)
		}
		if (digest != "") != tt.hasDigest {
			t.Errorf("parseImage(%q) digest=%q hasDigest=%v", tt.input, digest, tt.hasDigest)
		}
	}
}

// ── sanitizeImageRef ──

func TestSanitizeImageRef(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"nginx:latest", "nginx:latest"},
		{"nginx:latest@sha256:abc123", "nginx:latest@sha256:abc123"},
		{"nginx:latest@", "nginx:latest"},
		{"nginx:latest@baddigest", "nginx:latest"},
		{"nginx", "nginx"},
	}
	for _, tt := range tests {
		got := sanitizeImageRef(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeImageRef(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── serviceMode ──

func TestServiceMode(t *testing.T) {
	if m := serviceMode(nil); m != "" {
		t.Errorf("nil spec: got %q", m)
	}
}

// ── redactRegistryCreds ──

func TestRedactRegistryCreds(t *testing.T) {
	reg := map[string]any{"name": "test", "password": "secret", "token": "tok123", "url": "https://x.io"}
	redacted := redactRegistryCreds(reg)
	if redacted["password"] != "••••••" {
		t.Error("password not redacted")
	}
	if redacted["token"] != "••••••" {
		t.Error("token not redacted")
	}
	if redacted["name"] != "test" {
		t.Error("name should not be redacted")
	}
	// Original should not be modified
	if reg["password"] != "secret" {
		t.Error("original modified")
	}
}

// ── HealthLive ──

func TestHealthLive(t *testing.T) {
	req := httptest.NewRequest("GET", "/health/live", nil)
	rec := httptest.NewRecorder()
	HealthLive(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "UP" {
		t.Errorf("got %v", body)
	}
}

// ── Version ── (skipped: requires Docker connection)

// ── Initialize ──

func TestInitialize(t *testing.T) {
	setupAPI(t)
	payload := `{"username":"admin","password":"secret","email":"a@b.com"}`
	req := httptest.NewRequest("POST", "/initialize", bytes.NewBufferString(payload))
	rec := httptest.NewRecorder()
	Initialize(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Second call should fail
	req2 := httptest.NewRequest("POST", "/initialize", bytes.NewBufferString(payload))
	rec2 := httptest.NewRecorder()
	Initialize(rec2, req2)
	if rec2.Code != 400 {
		t.Errorf("expected 400 on duplicate init, got %d", rec2.Code)
	}
}

// ── Login ──

func TestLogin(t *testing.T) {
	setupAPI(t)
	store.CreateUser("testuser", "testpass", "admin", "")

	// Valid login
	req := httptest.NewRequest("POST", "/login", nil)
	req.Header.Set("Authorization", "Basic dGVzdHVzZXI6dGVzdHBhc3M=") // testuser:testpass
	rec := httptest.NewRecorder()
	Login(rec, req)
	if rec.Code != 200 {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["token"] == "" {
		t.Error("token should not be empty")
	}

	// Invalid login
	req2 := httptest.NewRequest("POST", "/login", nil)
	req2.Header.Set("Authorization", "Basic dGVzdHVzZXI6d3Jvbmc=") // testuser:wrong
	rec2 := httptest.NewRecorder()
	Login(rec2, req2)
	if rec2.Code != 401 {
		t.Errorf("expected 401, got %d", rec2.Code)
	}

	// No credentials
	req3 := httptest.NewRequest("POST", "/login", nil)
	rec3 := httptest.NewRecorder()
	Login(rec3, req3)
	if rec3.Code != 400 {
		t.Errorf("expected 400, got %d", rec3.Code)
	}
}

// ── ComposeValidate ──

func TestComposeValidate(t *testing.T) {
	// Valid YAML
	req := httptest.NewRequest("POST", "/api/compose/validate", bytes.NewBufferString(`version: "3.8"
services:
  web:
    image: nginx
`))
	rec := httptest.NewRecorder()
	ComposeValidate(rec, req)
	var body map[string]any
	json.NewDecoder(rec.Body).Decode(&body)
	if body["valid"] != true {
		t.Errorf("expected valid=true, got %v", body)
	}

	// Invalid YAML
	req2 := httptest.NewRequest("POST", "/api/compose/validate", bytes.NewBufferString(`{{{invalid`))
	rec2 := httptest.NewRecorder()
	ComposeValidate(rec2, req2)
	var body2 map[string]any
	json.NewDecoder(rec2.Body).Decode(&body2)
	if body2["valid"] != false {
		t.Errorf("expected valid=false, got %v", body2)
	}
}

// ── StackImport ──

func TestStackImport_BadInput(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/stacks/import", bytes.NewBufferString(`"not an array"`))
	rec := httptest.NewRecorder()
	StackImport(rec, req)
	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestStackImport_EmptyArray(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/stacks/import", bytes.NewBufferString(`[]`))
	rec := httptest.NewRecorder()
	StackImport(rec, req)
	if rec.Code != 400 {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ── json200 / jsonErr ──

func TestJson200(t *testing.T) {
	rec := httptest.NewRecorder()
	json200(rec, map[string]string{"key": "val"})
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("wrong content type")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["key"] != "val" {
		t.Error("body mismatch")
	}
}

func TestJsonErr(t *testing.T) {
	rec := httptest.NewRecorder()
	jsonErr(rec, 404, "not found")
	if rec.Code != 404 {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "not found" {
		t.Error("error mismatch")
	}
}

// ── stackCreatedAt / stackUpdatedAt ──

func TestStackTimestamps(t *testing.T) {
	svcs := []map[string]any{
		{"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-02T00:00:00Z"},
		{"createdAt": "2025-12-01T00:00:00Z", "updatedAt": "2026-01-05T00:00:00Z"},
	}
	created := stackCreatedAt(svcs)
	if created != "2025-12-01T00:00:00Z" {
		t.Errorf("createdAt=%v", created)
	}
	updated := stackUpdatedAt(svcs)
	if updated != "2026-01-05T00:00:00Z" {
		t.Errorf("updatedAt=%v", updated)
	}
	// Empty
	if stackCreatedAt(nil) != nil {
		t.Error("should be nil for empty")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
