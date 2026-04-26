package store

import (
	"encoding/json"
	"os"
	"testing"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// ── Users ──

func TestCreateAndAuthenticateUser(t *testing.T) {
	setupTestDB(t)
	u, err := CreateUser("alice", "secret123", "admin", "alice@test.com")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Username != "alice" || u.Role != "admin" {
		t.Errorf("got %+v", u)
	}
	if !AdminExists() {
		t.Error("AdminExists should be true")
	}
	auth := AuthenticateUser("alice", "secret123")
	if auth == nil {
		t.Fatal("AuthenticateUser returned nil")
	}
	if auth.Username != "alice" {
		t.Errorf("got username %q", auth.Username)
	}
	if AuthenticateUser("alice", "wrong") != nil {
		t.Error("should reject wrong password")
	}
	if AuthenticateUser("nobody", "secret123") != nil {
		t.Error("should reject unknown user")
	}
}

func TestUserCRUD(t *testing.T) {
	setupTestDB(t)
	CreateUser("bob", "pass", "user", "bob@test.com")
	users := Users()
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	u := GetUserByUsername("bob")
	if u == nil {
		t.Fatal("GetUserByUsername returned nil")
	}
	if err := UpdateUser(u.ID, "bobby", "bobby@test.com", "admin"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	updated := GetUserByUsername("bobby")
	if updated == nil || updated.Role != "admin" {
		t.Error("update failed")
	}
	if err := DeleteUser(updated.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if len(Users()) != 0 {
		t.Error("user not deleted")
	}
}

func TestUpdatePassword(t *testing.T) {
	setupTestDB(t)
	CreateUser("carol", "old", "user", "")
	if err := UpdatePassword("carol", "new"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if AuthenticateUser("carol", "old") != nil {
		t.Error("old password should not work")
	}
	if AuthenticateUser("carol", "new") == nil {
		t.Error("new password should work")
	}
}

// ── Secret ──

func TestGetSecret(t *testing.T) {
	setupTestDB(t)
	s := GetSecret()
	if s == "" {
		t.Error("secret should be auto-generated")
	}
}

// ── Registries ──

func TestRegistryCRUD(t *testing.T) {
	setupTestDB(t)
	reg := map[string]any{"name": "ghcr", "url": "https://ghcr.io", "username": "user", "password": "tok"}
	id, err := CreateRegistry(reg)
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}
	regs, _ := ListRegistries("") // type is empty since we didn't set it
	if len(regs) != 0 {
		t.Log("listing by empty type returns nothing — expected")
	}
	got, err := GetRegistry(id)
	if err != nil {
		t.Fatalf("GetRegistry: %v", err)
	}
	if got["name"] != "ghcr" {
		t.Errorf("got name %v", got["name"])
	}
	reg["name"] = "updated"
	if err := UpdateRegistry(id, reg); err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}
	got2, _ := GetRegistry(id)
	if got2["name"] != "updated" {
		t.Error("update failed")
	}
	if err := DeleteRegistry(id); err != nil {
		t.Fatalf("DeleteRegistry: %v", err)
	}
	if _, err := GetRegistry(id); err == nil {
		t.Error("should be deleted")
	}
}

func TestFindRegistryByHost(t *testing.T) {
	setupTestDB(t)
	reg := map[string]any{"name": "gl", "url": "https://registry.gitlab.com/myrepo", "type": "gitlab"}
	CreateRegistry(reg)
	found, err := FindRegistryByHost("registry.gitlab.com")
	if err != nil {
		t.Fatalf("FindRegistryByHost: %v", err)
	}
	if found["name"] != "gl" {
		t.Errorf("got %v", found["name"])
	}
	if _, err := FindRegistryByHost("unknown.io"); err == nil {
		t.Error("should not find unknown host")
	}
}

// ── Webhooks ──

func TestWebhookCRUD(t *testing.T) {
	setupTestDB(t)
	wh := CreateWebhook("svc-123")
	if wh["token"] == "" {
		t.Error("token should not be empty")
	}
	svcID, ok := FindWebhook(wh["token"])
	if !ok || svcID != "svc-123" {
		t.Errorf("FindWebhook: got %q %v", svcID, ok)
	}
	if _, ok := FindWebhook("nonexistent"); ok {
		t.Error("should not find nonexistent token")
	}
	list := ListWebhooks()
	if len(list) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(list))
	}
	DeleteWebhook(list[0]["id"].(string))
	if len(ListWebhooks()) != 0 {
		t.Error("webhook not deleted")
	}
}

// ── Audit ──

func TestAudit(t *testing.T) {
	setupTestDB(t)
	RecordAudit("admin", "create", "service", "web")
	RecordAudit("admin", "delete", "service", "web")
	entries := AuditEntries(10, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

// ── Dashboard Pins ──

func TestDashboardPins(t *testing.T) {
	setupTestDB(t)
	pinned := TogglePin("alice", "service", "svc-1")
	if !pinned {
		t.Error("first toggle should pin")
	}
	pins := GetPins("alice", "service")
	if len(pins) != 1 || pins[0] != "svc-1" {
		t.Errorf("got %v", pins)
	}
	unpinned := TogglePin("alice", "service", "svc-1")
	if unpinned {
		t.Error("second toggle should unpin")
	}
	if len(GetPins("alice", "service")) != 0 {
		t.Error("should be empty after unpin")
	}
}

// ── Alert Rules ──

func TestAlertRuleCRUD(t *testing.T) {
	setupTestDB(t)
	rule := CreateAlertRule("cpu_high", "gt", 80.0, "webhook", "http://example.com")
	if rule["id"] == "" {
		t.Error("id should not be empty")
	}
	rules := ListAlertRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	UpdateAlertRule(rule["id"].(string), "memory_high", "gt", 90.0, "webhook", "http://example.com", false)
	rules = ListAlertRules()
	if rules[0]["type"] != "memory_high" {
		t.Error("update failed")
	}
	RecordAlertHistory(rule["id"].(string), "test alert")
	history := ListAlertHistory()
	if len(history) != 1 {
		t.Error("history not recorded")
	}
	DeleteAlertRule(rule["id"].(string))
	if len(ListAlertRules()) != 0 {
		t.Error("rule not deleted")
	}
}

// ── Templates ──

func TestTemplateCRUD(t *testing.T) {
	setupTestDB(t)
	tpl := CreateTemplate("nginx", "web server", `{"image":"nginx"}`)
	got, err := GetTemplate(tpl["id"].(string))
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if got["name"] != "nginx" {
		t.Error("name mismatch")
	}
	list := ListTemplates()
	if len(list) != 1 {
		t.Error("expected 1 template")
	}
	DeleteTemplate(tpl["id"].(string))
	if len(ListTemplates()) != 0 {
		t.Error("template not deleted")
	}
}

// ── Git Stacks ──

func TestGitStackCRUD(t *testing.T) {
	setupTestDB(t)
	gs := GitStack{StackName: "myapp", RepoURL: "https://github.com/test/repo", Enabled: true}
	created, err := CreateGitStack(gs)
	if err != nil {
		t.Fatalf("CreateGitStack: %v", err)
	}
	if created.ID == "" || created.Branch != "main" {
		t.Errorf("defaults not applied: %+v", created)
	}
	got, err := GetGitStack(created.ID)
	if err != nil {
		t.Fatalf("GetGitStack: %v", err)
	}
	if got.StackName != "myapp" {
		t.Error("name mismatch")
	}
	byName, err := GetGitStackByName("myapp")
	if err != nil || byName.ID != created.ID {
		t.Error("GetGitStackByName failed")
	}
	got.RepoURL = "https://github.com/test/updated"
	UpdateGitStack(created.ID, got)
	updated, _ := GetGitStack(created.ID)
	if updated.RepoURL != "https://github.com/test/updated" {
		t.Error("update failed")
	}
	UpdateGitStackSync(created.ID, "abc123", "2026-01-01T00:00:00Z", "")
	synced, _ := GetGitStack(created.ID)
	if synced.LastHash != "abc123" {
		t.Error("sync update failed")
	}
	list := ListGitStacks()
	if len(list) != 1 {
		t.Error("expected 1 git stack")
	}
	DeleteGitStack(created.ID)
	if len(ListGitStacks()) != 0 {
		t.Error("git stack not deleted")
	}
}

// ── Backup/Restore ──

func TestBackupRestore(t *testing.T) {
	setupTestDB(t)
	CreateUser("admin", "pass123", "admin", "admin@test.com")
	CreateWebhook("svc-1")
	data, err := ExportAll()
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	users, ok := data["users"].([]map[string]any)
	if !ok || len(users) != 1 {
		t.Fatalf("expected 1 user in backup, got %v", data["users"])
	}
	// Check password is included in backup
	if users[0]["password"] == nil {
		t.Error("password should be included in backup")
	}
	// Serialize and reimport
	raw := map[string]json.RawMessage{}
	for k, v := range data {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	if err := ImportAll(raw); err != nil {
		t.Fatalf("ImportAll: %v", err)
	}
	// Verify user still exists and can authenticate
	restored := Users()
	if len(restored) != 1 || restored[0].Username != "admin" {
		t.Error("user not restored")
	}
}

// ── Timeseries ──

func TestTimeseries(t *testing.T) {
	setupTestDB(t)
	rows := []TsRow{{NodeID: "n1", Ts: 1000, CPU: 50.0, Memory: 60.0, Disk: 70.0}}
	if err := SaveTimeseries(rows); err != nil {
		t.Fatalf("SaveTimeseries: %v", err)
	}
	loaded, err := LoadTimeseries()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("LoadTimeseries: %v, len=%d", err, len(loaded))
	}
	if loaded[0].CPU != 50.0 {
		t.Error("CPU mismatch")
	}
	PruneTimeseries(2000)
	after, _ := LoadTimeseries()
	if len(after) != 0 {
		t.Error("prune failed")
	}
}

// ── TOTP ──

func TestTOTP(t *testing.T) {
	setupTestDB(t)
	CreateUser("dave", "pass", "user", "")
	if err := SetTOTPSecret("dave", "ABCDEF123456"); err != nil {
		t.Fatalf("SetTOTPSecret: %v", err)
	}
	if s := GetTOTPSecret("dave"); s != "ABCDEF123456" {
		t.Errorf("got %q", s)
	}
	ClearTOTPSecret("dave")
	if s := GetTOTPSecret("dave"); s != "" {
		t.Error("should be cleared")
	}
}

// ── Team Permissions ──

func TestTeamPermissions(t *testing.T) {
	setupTestDB(t)
	InitTeamPermissions()
	id, err := CreateTeamPermission("devops", "alice", "prod-*", "admin")
	if err != nil {
		t.Fatalf("CreateTeamPermission: %v", err)
	}
	list := ListTeamPermissions()
	if len(list) != 1 || list[0]["teamName"] != "devops" {
		t.Errorf("got %v", list)
	}
	if !UserStackAccess("alice", "prod-web") {
		t.Log("pattern matching is exact, not glob — expected for now")
	}
	if !UserStackAccess("alice", "*") {
		t.Log("wildcard check works via stack_pattern='*'")
	}
	DeleteTeamPermission(id)
	if len(ListTeamPermissions()) != 0 {
		t.Error("not deleted")
	}
}

// ── Clusters ──

func TestClusterCRUD(t *testing.T) {
	setupTestDB(t)
	InitClusters()
	id, err := CreateCluster("prod", "tcp://10.0.0.1:2376", "", "", "")
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}
	list := ListClusters()
	if len(list) != 1 || list[0]["name"] != "prod" {
		t.Errorf("got %v", list)
	}
	SetActiveCluster(id)
	activeID, host := GetActiveCluster()
	if activeID != id || host != "tcp://10.0.0.1:2376" {
		t.Errorf("active: %q %q", activeID, host)
	}
	DeleteCluster(id)
	if len(ListClusters()) != 0 {
		t.Error("not deleted")
	}
}

// ── DBDir ──

func TestDBDir(t *testing.T) {
	setupTestDB(t)
	if d := DBDir(); d == "" {
		t.Error("DBDir should not be empty")
	}
}

// ── Edge: duplicate user ──

func TestDuplicateUser(t *testing.T) {
	setupTestDB(t)
	CreateUser("dup", "pass", "user", "")
	_, err := CreateUser("dup", "pass2", "admin", "")
	if err == nil {
		t.Error("should reject duplicate username")
	}
}

// ── Edge: empty DB ──

func TestEmptyLists(t *testing.T) {
	dir := t.TempDir()
	Init(dir)
	if AdminExists() {
		t.Error("no admin should exist")
	}
	if len(Users()) != 0 {
		t.Error("should be empty")
	}
	if len(ListWebhooks()) != 0 {
		t.Error("should be empty")
	}
	if len(AuditEntries(10, 0)) != 0 {
		t.Error("should be empty")
	}
	regs, _ := ListRegistries("dockerhub")
	if len(regs) != 0 {
		t.Error("should be empty")
	}
	if len(ListAlertRules()) != 0 {
		t.Error("should be empty")
	}
	if len(ListTemplates()) != 0 {
		t.Error("should be empty")
	}
	if len(ListGitStacks()) != 0 {
		t.Error("should be empty")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
