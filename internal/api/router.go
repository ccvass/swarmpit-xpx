package api

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/ccvass/swarmpit-xpx/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(staticFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Backend-Server", "swarmpit")
			next.ServeHTTP(w, req)
		})
	})
	// Inject router into context for swagger route generation
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			ctx = context.WithValue(ctx, chiRouterKey, r)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	// Read index.html and inject CSS/JS tags
	rawIndex, _ := fs.ReadFile(staticFS, "index.html")
	idx := strings.Replace(string(rawIndex), "</head>", `<link rel="stylesheet" href="/css/main.css?v=2.5.0"></head>`, 1)
	idx = strings.Replace(idx, "</body>", `<script src="/js/main.js?v=2.5.0"></script></body>`, 1)
	indexHTML := []byte(idx)

	// Public
	r.Get("/version", Version)
	r.Get("/health/live", HealthLive)
	r.Get("/health/ready", HealthReady)
	r.Get("/api-docs", SwaggerUI)
	r.Get("/api-docs/swagger.json", SwaggerJSON)
	r.Post("/login", Login)
	r.Post("/initialize", Initialize)
	r.Post("/api/webhooks/{token}", WebhookTrigger)
	r.Get("/events", SSEHandler)
	r.Post("/events", EventPush)
	r.Get("/slt", func(w http.ResponseWriter, r *http.Request) {
		json200(w, map[string]string{"slt": "go-backend-slt"})
	})

	// Authenticated
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)

		r.Get("/api/nodes", NodeList)
		r.Get("/api/nodes/ts", NodeTimeseries)
		r.Get("/api/nodes/{id}", NodeDetail)
		r.Get("/api/nodes/{id}/tasks", NodeTasks)
		r.Get("/api/stats", Stats)
		r.Get("/api/services", ServiceList)
		r.Get("/api/services/ts/cpu", ServicesTsCPU)
		r.Get("/api/services/ts/memory", ServicesTsMemory)
		r.Get("/api/services/{id}", ServiceInfo)
		r.Get("/api/services/{id}/logs", ServiceLogs)
		r.Get("/api/services/{id}/tasks", ServiceTaskList)
		r.Get("/api/services/{id}/networks", ServiceNetworks)
		r.Get("/api/services/{id}/compose", ServiceCompose)
		r.Get("/api/services/{id}/ts", ServiceTimeseries)
		r.Delete("/api/services/{id}", ServiceDelete)
		r.Post("/api/services", ServiceCreate)
		r.Post("/api/services/{id}/update", ServiceUpdate)
		r.Post("/api/services/{id}/redeploy", ServiceRedeploy)
		r.Post("/api/services/{id}/rollback", ServiceRollback)
		r.Get("/api/tasks", TaskList)
		r.Get("/api/tasks/{id}", TaskInfo)
		r.Get("/api/tasks/{id}/ts", TaskTimeseries)
		r.Get("/api/networks", NetworkList)
		r.Get("/api/networks/{id}", NetworkInfo)
		r.Get("/api/networks/{id}/services", NetworkServices)
		r.Delete("/api/networks/{id}", NetworkDelete)
		r.Post("/api/networks", NetworkCreate)
		r.Get("/api/volumes", VolumeList)
		r.Get("/api/volumes/{id}", VolumeInfo)
		r.Get("/api/volumes/{id}/services", VolumeServices)
		r.Post("/api/volumes", VolumeCreate)
		r.Get("/api/secrets", SecretList)
		r.Get("/api/secrets/{id}", SecretInfo)
		r.Get("/api/secrets/{id}/services", SecretServices)
		r.Delete("/api/secrets/{id}", SecretDelete)
		r.Post("/api/secrets", SecretCreate)
		r.Put("/api/secrets/{id}", SecretUpdate)
		r.Get("/api/configs", ConfigList)
		r.Get("/api/configs/{id}", ConfigInfo)
		r.Get("/api/configs/{id}/services", ConfigServices)
		r.Delete("/api/configs/{id}", ConfigDelete)
		r.Post("/api/configs", ConfigCreate)
		r.Put("/api/configs/{id}", ConfigUpdate)
		r.Get("/api/labels/service", LabelsService)
		r.Get("/api/plugin/network", PluginNetwork)
		r.Get("/api/plugin/volume", PluginVolume)
		r.Get("/api/plugin/log", PluginLog)
		r.Get("/api/placement", Placement)
		r.Get("/api/stacks", StackList)
		r.Get("/api/stacks/{name}", StackInfo)
		r.Get("/api/stacks/{name}/services", StackServices)
		r.Get("/api/stacks/{name}/tasks", StackTasks)
		r.Get("/api/stacks/{name}/networks", StackNetworks)
		r.Get("/api/stacks/{name}/volumes", StackVolumes)
		r.Get("/api/stacks/{name}/configs", StackConfigs)
		r.Get("/api/stacks/{name}/secrets", StackSecrets)
		r.Get("/api/stacks/{name}/file", StackFile)
		r.Get("/api/stacks/{name}/compose", StackCompose)
		r.Post("/api/stacks/git", GitDeploy)
		r.Post("/api/stacks", StackCreate)
		r.Put("/api/stacks/{name}", StackUpdate)
		r.Delete("/api/stacks/{name}", StackDelete)
		r.Post("/api/stacks/{name}/redeploy", StackRedeploy)
		r.Post("/api/stacks/{name}/rollback", StackRollback)
		r.Post("/api/services/{id}/stop", ServiceStop)
		r.Put("/api/nodes/{id}", NodeEdit)
		r.Post("/api/stacks/{name}/activate", StackActivate)
		r.Post("/api/stacks/{name}/deactivate", StackDeactivate)
		r.Get("/api/me", Me)
		r.Post("/api/password", PasswordChange)
		r.Get("/api/webhooks", WebhookList)
		r.Post("/api/webhooks", WebhookCreate)
		r.Delete("/api/webhooks/{id}", WebhookDelete)
		r.Get("/api/registry/{type}", RegistryList)
		r.Post("/api/registry/{type}", RegistryCreate)
		r.Get("/api/registry/{type}/{id}", RegistryInfo)
		r.Put("/api/registry/{type}/{id}", RegistryUpdate)
		r.Delete("/api/registry/{type}/{id}", RegistryDelete)
		r.Get("/api/registry/{type}/{id}/repositories", RegistryRepositories)
		r.Get("/api/public/repositories", PublicRepositories)
		r.Get("/api/tags/*", ImageTags)
		r.Get("/exec/{id}", ExecHandler)

		r.Post("/api/services/{id}/dashboard", DashboardPinService)
		r.Post("/api/nodes/{id}/dashboard", DashboardPinNode)

		r.Get("/api/alerts", AlertRuleList)
		r.Post("/api/alerts", AlertRuleCreate)
		r.Put("/api/alerts/{id}", AlertRuleUpdate)
		r.Delete("/api/alerts/{id}", AlertRuleDelete)
		r.Get("/api/alerts/history", AlertHistoryList)

		r.Get("/api/templates", TemplateList)
		r.Post("/api/templates", TemplateCreate)
		r.Post("/api/templates/{id}/deploy", TemplateDeploy)
		r.Delete("/api/templates/{id}", TemplateDelete)

		r.Post("/api/compose/validate", ComposeValidate)

		r.Group(func(r chi.Router) {
			r.Use(auth.AdminOnly)
			r.Get("/api/audit", AuditList)
			r.Get("/api/users", UserList)
			r.Post("/api/users", UserCreate)
			r.Put("/api/users/{id}", UserUpdate)
			r.Delete("/api/users/{id}", UserDelete)
			r.Get("/api/backup", BackupHandler)
			r.Post("/api/restore", RestoreHandler)
		})
	})

	// Static files + SPA fallback
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[1:]
		if path == "" {
			w.Header().Set("Content-Type", "text/html")
			w.Write(indexHTML)
			return
		}
		f, err := staticFS.Open(path)
		if err != nil {
			// SPA fallback
			w.Header().Set("Content-Type", "text/html")
			w.Write(indexHTML)
			return
		}
		defer f.Close()
		stat, _ := f.Stat()
		if stat.IsDir() {
			w.Header().Set("Content-Type", "text/html")
			w.Write(indexHTML)
			return
		}
		// Serve the static file
		rs, ok := f.(io.ReadSeeker)
		if ok {
			http.ServeContent(w, r, path, stat.ModTime(), rs)
		} else {
			data, _ := io.ReadAll(f)
			w.Write(data)
		}
	})

	return r
}
