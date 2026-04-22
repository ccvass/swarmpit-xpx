package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB
var dbDir string

func Init(dbPath string) error {
	dbDir = dbPath
	os.MkdirAll(dbPath, 0755)
	var err error
	db, err = sql.Open("sqlite3", filepath.Join(dbPath, "swarmpit.db"))
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'admin',
			email TEXT, api_token TEXT
		);
		CREATE TABLE IF NOT EXISTS secret (id TEXT PRIMARY KEY, secret TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY, service_id TEXT NOT NULL, token TEXT UNIQUE NOT NULL,
			created_at INTEGER DEFAULT (strftime('%s','now')), last_triggered INTEGER
		);
		CREATE TABLE IF NOT EXISTS audit_log (
			id TEXT PRIMARY KEY, timestamp INTEGER DEFAULT (strftime('%s','now')),
			username TEXT, action TEXT, resource_type TEXT, resource_name TEXT
		);
		CREATE TABLE IF NOT EXISTS stats_ts (
			node_id TEXT, ts INTEGER, cpu REAL, memory REAL, disk REAL
		);
		CREATE TABLE IF NOT EXISTS registries (
			id TEXT PRIMARY KEY, type TEXT NOT NULL, name TEXT, url TEXT, data TEXT
		);
		CREATE TABLE IF NOT EXISTS dashboard_pins (
			username TEXT, resource_type TEXT, resource_id TEXT,
			PRIMARY KEY(username, resource_type, resource_id)
		);
		CREATE TABLE IF NOT EXISTS alert_rules (
			id TEXT PRIMARY KEY, type TEXT, condition TEXT, threshold REAL,
			channel TEXT, target TEXT, enabled INTEGER DEFAULT 1
		);
		CREATE TABLE IF NOT EXISTS alert_history (
			id TEXT PRIMARY KEY, rule_id TEXT, message TEXT, created_at TEXT
		);
		CREATE TABLE IF NOT EXISTS templates (
			id TEXT PRIMARY KEY, name TEXT, description TEXT, spec TEXT, created_at TEXT
		);
	`)
	if err != nil {
		return err
	}
	// Ensure JWT secret exists
	var count int
	db.QueryRow("SELECT COUNT(*) FROM secret").Scan(&count)
	if count == 0 {
		db.Exec("INSERT INTO secret (id, secret) VALUES (?, ?)", uuid.NewString(), uuid.NewString())
	}
	return nil
}

func GetSecret() string {
	var s string
	db.QueryRow("SELECT secret FROM secret LIMIT 1").Scan(&s)
	return s
}

// Users

type User struct {
	ID       string `json:"_id"`
	Username string `json:"username"`
	Password string `json:"-"`
	Role     string `json:"role"`
	Email    string `json:"email,omitempty"`
}

func CreateUser(username, password, role, email string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	_, err = db.Exec("INSERT INTO users (id, username, password, role, email) VALUES (?,?,?,?,?)",
		id, username, string(hash), role, email)
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Username: username, Role: role, Email: email}, nil
}

func AuthenticateUser(username, password string) *User {
	var u User
	var hash string
	err := db.QueryRow("SELECT id, username, password, role, email FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &hash, &u.Role, &u.Email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil
	}
	return &u
}

func AdminExists() bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	return count > 0
}

func Users() []User {
	rows, err := db.Query("SELECT id, username, role, email FROM users")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.Role, &u.Email)
		users = append(users, u)
	}
	return users
}

func ListUsers() []User { return Users() }

func UpdateUser(id, username, email, role string) error {
	_, err := db.Exec("UPDATE users SET username=?, email=?, role=? WHERE id=?", username, email, role, id)
	return err
}

func DeleteUser(id string) error {
	_, err := db.Exec("DELETE FROM users WHERE id=?", id)
	return err
}

// Dashboard pins

func TogglePin(username, resourceType, resourceID string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM dashboard_pins WHERE username=? AND resource_type=? AND resource_id=?",
		username, resourceType, resourceID).Scan(&count)
	if count > 0 {
		db.Exec("DELETE FROM dashboard_pins WHERE username=? AND resource_type=? AND resource_id=?",
			username, resourceType, resourceID)
		return false
	}
	db.Exec("INSERT INTO dashboard_pins (username, resource_type, resource_id) VALUES (?,?,?)",
		username, resourceType, resourceID)
	return true
}

func GetPins(username, resourceType string) []string {
	rows, err := db.Query("SELECT resource_id FROM dashboard_pins WHERE username=? AND resource_type=?",
		username, resourceType)
	if err != nil { return []string{} }
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if ids == nil { ids = []string{} }
	return ids
}

// Audit

func RecordAudit(username, action, resourceType, resourceName string) {
	db.Exec("INSERT INTO audit_log (id, username, action, resource_type, resource_name) VALUES (?,?,?,?,?)",
		uuid.NewString(), username, action, resourceType, resourceName)
}

func AuditEntries(limit, offset int) []map[string]any {
	rows, err := db.Query("SELECT id, timestamp, username, action, resource_type, resource_name FROM audit_log ORDER BY timestamp DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var entries []map[string]any
	for rows.Next() {
		var id, username, action, rt, rn string
		var ts int64
		rows.Scan(&id, &ts, &username, &action, &rt, &rn)
		entries = append(entries, map[string]any{"id": id, "timestamp": ts, "username": username, "action": action, "resource_type": rt, "resource_name": rn})
	}
	if entries == nil {
		entries = []map[string]any{}
	}
	return entries
}

// Webhooks

func CreateWebhook(serviceID string) map[string]string {
	token := uuid.NewString()
	db.Exec("INSERT INTO webhooks (id, service_id, token) VALUES (?,?,?)", uuid.NewString(), serviceID, token)
	return map[string]string{"token": token, "service-id": serviceID}
}

func FindWebhook(token string) (string, bool) {
	var serviceID string
	err := db.QueryRow("SELECT service_id FROM webhooks WHERE token = ?", token).Scan(&serviceID)
	if err != nil {
		return "", false
	}
	db.Exec("UPDATE webhooks SET last_triggered = strftime('%s','now') WHERE token = ?", token)
	return serviceID, true
}

// Generic JSON document store (for registries etc)

func SaveDoc(docType, id string, data any) error {
	j, _ := json.Marshal(data)
	_, err := db.Exec("INSERT OR REPLACE INTO documents (id, type, data) VALUES (?,?,?)", id, docType, string(j))
	return err
}

// Timeseries persistence

type TsRow struct {
	NodeID string
	Ts     int64
	CPU    float64
	Memory float64
	Disk   float64
}

func LoadTimeseries() ([]TsRow, error) {
	rows, err := db.Query("SELECT node_id, ts, cpu, memory, disk FROM stats_ts ORDER BY ts ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TsRow
	for rows.Next() {
		var r TsRow
		rows.Scan(&r.NodeID, &r.Ts, &r.CPU, &r.Memory, &r.Disk)
		result = append(result, r)
	}
	return result, nil
}

func SaveTimeseries(rows []TsRow) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO stats_ts (node_id, ts, cpu, memory, disk) VALUES (?,?,?,?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		stmt.Exec(r.NodeID, r.Ts, r.CPU, r.Memory, r.Disk)
	}
	return tx.Commit()
}

func PruneTimeseries(before int64) {
	db.Exec("DELETE FROM stats_ts WHERE ts < ?", before)
}

// ── Registry CRUD ──

func ListRegistries(regType string) ([]map[string]any, error) {
	rows, err := db.Query("SELECT id, type, name, url, data FROM registries WHERE type = ?", regType)
	if err != nil { return []map[string]any{}, err }
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, rtype, name, url, data string
		rows.Scan(&id, &rtype, &name, &url, &data)
		reg := map[string]any{"id": id, "type": rtype, "name": name, "url": url}
		var extra map[string]any
		if json.Unmarshal([]byte(data), &extra) == nil {
			for k, v := range extra { reg[k] = v }
		}
		result = append(result, reg)
	}
	if result == nil { result = []map[string]any{} }
	return result, nil
}

func CreateRegistry(reg map[string]any) (string, error) {
	id := fmt.Sprintf("reg-%d", time.Now().UnixNano())
	name, _ := reg["name"].(string)
	url, _ := reg["url"].(string)
	rtype, _ := reg["type"].(string)
	data, _ := json.Marshal(reg)
	_, err := db.Exec("INSERT INTO registries (id, type, name, url, data) VALUES (?, ?, ?, ?, ?)",
		id, rtype, name, url, string(data))
	return id, err
}

func GetRegistry(id string) (map[string]any, error) {
	var rtype, name, url, data string
	err := db.QueryRow("SELECT type, name, url, data FROM registries WHERE id = ?", id).Scan(&rtype, &name, &url, &data)
	if err != nil { return nil, err }
	reg := map[string]any{"id": id, "type": rtype, "name": name, "url": url}
	var extra map[string]any
	if json.Unmarshal([]byte(data), &extra) == nil {
		for k, v := range extra { reg[k] = v }
	}
	return reg, nil
}

func UpdateRegistry(id string, reg map[string]any) error {
	name, _ := reg["name"].(string)
	url, _ := reg["url"].(string)
	data, _ := json.Marshal(reg)
	_, err := db.Exec("UPDATE registries SET name=?, url=?, data=? WHERE id=?", name, url, string(data), id)
	return err
}

func DeleteRegistry(id string) error {
	_, err := db.Exec("DELETE FROM registries WHERE id = ?", id)
	return err
}

// ── User profile & password ──

func GetUserByUsername(username string) *User {
	var u User
	err := db.QueryRow("SELECT id, username, role, email FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.Role, &u.Email)
	if err != nil {
		return nil
	}
	return &u
}

func UpdatePassword(username, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE users SET password = ? WHERE username = ?", string(hash), username)
	return err
}

// ── Webhooks CRUD ──

func ListWebhooks() []map[string]any {
	rows, err := db.Query("SELECT id, service_id, token, created_at, last_triggered FROM webhooks ORDER BY created_at DESC")
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, svcID, token string
		var createdAt int64
		var lastTriggered sql.NullInt64
		rows.Scan(&id, &svcID, &token, &createdAt, &lastTriggered)
		entry := map[string]any{"id": id, "serviceName": svcID, "token": token, "createdAt": createdAt}
		if lastTriggered.Valid {
			entry["lastTriggered"] = lastTriggered.Int64
		}
		result = append(result, entry)
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func DeleteWebhook(id string) error {
	_, err := db.Exec("DELETE FROM webhooks WHERE id = ?", id)
	return err
}

// ── Alert rules CRUD ──

func ListAlertRules() []map[string]any {
	rows, err := db.Query("SELECT id, type, condition, threshold, channel, target, enabled FROM alert_rules")
	if err != nil { return []map[string]any{} }
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, typ, cond, ch, tgt string
		var threshold float64
		var enabled int
		rows.Scan(&id, &typ, &cond, &threshold, &ch, &tgt, &enabled)
		result = append(result, map[string]any{"id": id, "type": typ, "condition": cond, "threshold": threshold, "channel": ch, "target": tgt, "enabled": enabled == 1})
	}
	if result == nil { result = []map[string]any{} }
	return result
}

func CreateAlertRule(typ, cond string, threshold float64, ch, tgt string) map[string]any {
	id := uuid.NewString()
	db.Exec("INSERT INTO alert_rules (id, type, condition, threshold, channel, target, enabled) VALUES (?,?,?,?,?,?,1)", id, typ, cond, threshold, ch, tgt)
	return map[string]any{"id": id, "type": typ, "condition": cond, "threshold": threshold, "channel": ch, "target": tgt, "enabled": true}
}

func UpdateAlertRule(id string, typ, cond string, threshold float64, ch, tgt string, enabled bool) error {
	e := 0; if enabled { e = 1 }
	_, err := db.Exec("UPDATE alert_rules SET type=?, condition=?, threshold=?, channel=?, target=?, enabled=? WHERE id=?", typ, cond, threshold, ch, tgt, e, id)
	return err
}

func DeleteAlertRule(id string) error {
	_, err := db.Exec("DELETE FROM alert_rules WHERE id=?", id)
	return err
}

func ListAlertHistory() []map[string]any {
	rows, err := db.Query("SELECT id, rule_id, message, created_at FROM alert_history ORDER BY created_at DESC LIMIT 100")
	if err != nil { return []map[string]any{} }
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, ruleID, msg, createdAt string
		rows.Scan(&id, &ruleID, &msg, &createdAt)
		result = append(result, map[string]any{"id": id, "rule_id": ruleID, "message": msg, "created_at": createdAt})
	}
	if result == nil { result = []map[string]any{} }
	return result
}

func RecordAlertHistory(ruleID, message string) {
	db.Exec("INSERT INTO alert_history (id, rule_id, message, created_at) VALUES (?,?,?,?)", uuid.NewString(), ruleID, message, time.Now().UTC().Format(time.RFC3339))
}

// ── Templates CRUD ──

func ListTemplates() []map[string]any {
	rows, err := db.Query("SELECT id, name, description, spec, created_at FROM templates ORDER BY created_at DESC")
	if err != nil { return []map[string]any{} }
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, name, desc, spec, createdAt string
		rows.Scan(&id, &name, &desc, &spec, &createdAt)
		result = append(result, map[string]any{"id": id, "name": name, "description": desc, "spec": spec, "created_at": createdAt})
	}
	if result == nil { result = []map[string]any{} }
	return result
}

func CreateTemplate(name, desc, spec string) map[string]any {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO templates (id, name, description, spec, created_at) VALUES (?,?,?,?,?)", id, name, desc, spec, now)
	return map[string]any{"id": id, "name": name, "description": desc, "spec": spec, "created_at": now}
}

func GetTemplate(id string) (map[string]any, error) {
	var name, desc, spec, createdAt string
	err := db.QueryRow("SELECT name, description, spec, created_at FROM templates WHERE id=?", id).Scan(&name, &desc, &spec, &createdAt)
	if err != nil { return nil, err }
	return map[string]any{"id": id, "name": name, "description": desc, "spec": spec, "created_at": createdAt}, nil
}

func DeleteTemplate(id string) error {
	_, err := db.Exec("DELETE FROM templates WHERE id=?", id)
	return err
}

// ── Backup/Restore (#55) ──

func ExportAll() (map[string]any, error) {
	result := map[string]any{}
	tables := map[string]string{
		"users":          "SELECT id, username, role, email FROM users",
		"registries":     "SELECT id, type, name, url, data FROM registries",
		"webhooks":       "SELECT id, service_id, token, created_at, last_triggered FROM webhooks",
		"alert_rules":    "SELECT id, type, condition, threshold, channel, target, enabled FROM alert_rules",
		"templates":      "SELECT id, name, description, spec, created_at FROM templates",
		"dashboard_pins": "SELECT username, resource_type, resource_id FROM dashboard_pins",
	}
	for name, query := range tables {
		rows, err := db.Query(query)
		if err != nil {
			continue
		}
		cols, _ := rows.Columns()
		var records []map[string]any
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows.Scan(ptrs...)
			row := map[string]any{}
			for i, c := range cols {
				row[c] = vals[i]
			}
			records = append(records, row)
		}
		rows.Close()
		if records == nil {
			records = []map[string]any{}
		}
		result[name] = records
	}
	return result, nil
}

func ImportAll(data map[string]json.RawMessage) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	type tableConf struct {
		clear  string
		insert string
		cols   []string
	}
	tables := map[string]tableConf{
		"users":          {"DELETE FROM users", "INSERT OR REPLACE INTO users (id, username, role, email) VALUES (?,?,?,?)", []string{"id", "username", "role", "email"}},
		"registries":     {"DELETE FROM registries", "INSERT OR REPLACE INTO registries (id, type, name, url, data) VALUES (?,?,?,?,?)", []string{"id", "type", "name", "url", "data"}},
		"webhooks":       {"DELETE FROM webhooks", "INSERT OR REPLACE INTO webhooks (id, service_id, token, created_at, last_triggered) VALUES (?,?,?,?,?)", []string{"id", "service_id", "token", "created_at", "last_triggered"}},
		"alert_rules":    {"DELETE FROM alert_rules", "INSERT OR REPLACE INTO alert_rules (id, type, condition, threshold, channel, target, enabled) VALUES (?,?,?,?,?,?,?)", []string{"id", "type", "condition", "threshold", "channel", "target", "enabled"}},
		"templates":      {"DELETE FROM templates", "INSERT OR REPLACE INTO templates (id, name, description, spec, created_at) VALUES (?,?,?,?,?)", []string{"id", "name", "description", "spec", "created_at"}},
		"dashboard_pins": {"DELETE FROM dashboard_pins", "INSERT OR REPLACE INTO dashboard_pins (username, resource_type, resource_id) VALUES (?,?,?)", []string{"username", "resource_type", "resource_id"}},
	}

	for name, conf := range tables {
		raw, ok := data[name]
		if !ok {
			continue
		}
		var records []map[string]any
		if err := json.Unmarshal(raw, &records); err != nil {
			continue
		}
		tx.Exec(conf.clear)
		stmt, err := tx.Prepare(conf.insert)
		if err != nil {
			continue
		}
		for _, rec := range records {
			vals := make([]any, len(conf.cols))
			for i, c := range conf.cols {
				vals[i] = rec[c]
			}
			stmt.Exec(vals...)
		}
		stmt.Close()
	}
	return tx.Commit()
}
