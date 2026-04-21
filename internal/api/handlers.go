package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ccvass/swarmpit-xpx/internal/auth"
	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
	"github.com/go-chi/chi/v5"
)

func json200(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Version

func Version(w http.ResponseWriter, r *http.Request) {
	ver, _ := docker.Version()
	ping, _ := docker.Ping()
	json200(w, map[string]any{
		"name": "swarmpit", "version": "2.0.0-go",
		"initialized": store.AdminExists(), "statistics": true,
		"docker": map[string]any{"api": ping.APIVersion, "engine": ver.Version},
	})
}

// Health

func HealthLive(w http.ResponseWriter, r *http.Request) {
	json200(w, map[string]string{"status": "UP"})
}

func HealthReady(w http.ResponseWriter, r *http.Request) {
	_, dockerErr := docker.Ping()
	status := "UP"
	code := http.StatusOK
	if dockerErr != nil {
		status = "DOWN"
		code = http.StatusServiceUnavailable
	}
	w.WriteHeader(code)
	json200(w, map[string]any{"status": status, "components": map[string]string{
		"docker": status, "sqlite": "UP", "stats": "in-memory",
	}})
}

// Auth

func Login(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	username, password, ok := auth.DecodeBasic(authHeader)
	if !ok {
		jsonErr(w, 400, "Missing credentials")
		return
	}
	user := store.AuthenticateUser(username, password)
	if user == nil {
		jsonErr(w, 401, "Invalid credentials")
		return
	}
	token, _ := auth.GenerateJWT(user.Username, user.Role)
	json200(w, map[string]string{"token": "Bearer " + token})
}

func Initialize(w http.ResponseWriter, r *http.Request) {
	if store.AdminExists() {
		jsonErr(w, 400, "Admin already exists")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	user, err := store.CreateUser(body.Username, body.Password, "admin", body.Email)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	json200(w, user)
}

// Docker resources

func NodeList(w http.ResponseWriter, r *http.Request) {
	nodes, err := docker.Nodes()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, nodes)
}

func NodeInfo(w http.ResponseWriter, r *http.Request) {
	node, err := docker.Node(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	json200(w, node)
}

func ServiceList(w http.ResponseWriter, r *http.Request) {
	svcs, err := docker.Services()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, svcs)
}

func ServiceInfo(w http.ResponseWriter, r *http.Request) {
	svc, err := docker.Service(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	json200(w, svc)
}

func TaskList(w http.ResponseWriter, r *http.Request) {
	tasks, err := docker.Tasks()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, tasks)
}

func NetworkList(w http.ResponseWriter, r *http.Request) {
	nets, err := docker.Networks()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, nets)
}

func SecretList(w http.ResponseWriter, r *http.Request) {
	secrets, err := docker.Secrets()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, secrets)
}

func ConfigList(w http.ResponseWriter, r *http.Request) {
	configs, err := docker.Configs()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, configs)
}

// Webhooks

func WebhookTrigger(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	serviceID, ok := store.FindWebhook(token)
	if !ok {
		jsonErr(w, 404, "Webhook not found")
		return
	}
	svc, err := docker.Service(serviceID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	force := svc.Spec.TaskTemplate.ForceUpdate + 1
	svc.Spec.TaskTemplate.ForceUpdate = force
	// TODO: call docker.UpdateService
	json200(w, map[string]string{"status": "triggered"})
}

// Audit

func AuditList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit == 0 {
		limit = 50
	}
	json200(w, store.AuditEntries(limit, offset))
}
