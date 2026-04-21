package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

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
