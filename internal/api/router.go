package api

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/ccvass/swarmpit-xpx/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/time/rate"
)

// ipRateLimiter tracks per-IP rate limiters
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newIPRateLimiter(r rate.Limit, b int) *ipRateLimiter {
	return &ipRateLimiter{limiters: make(map[string]*rate.Limiter), rate: r, burst: b}
}

func (i *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()
	if lim, ok := i.limiters[ip]; ok {
		return lim
	}
	lim := rate.NewLimiter(i.rate, i.burst)
	i.limiters[ip] = lim
	return lim
}

func rateLimitMiddleware(lim *ipRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = strings.SplitN(fwd, ",", 2)[0]
			}
			if !lim.getLimiter(strings.TrimSpace(ip)).Allow() {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

var (
	// 100 req/min general, 5 req/min for login
	apiLimiter   = newIPRateLimiter(rate.Limit(20), 50)
	loginLimiter = newIPRateLimiter(rate.Limit(5.0/60.0), 5)
)

func NewRouter(staticFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(rateLimitMiddleware(apiLimiter))
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
	idx := strings.Replace(string(rawIndex), "</head>", `<link rel="stylesheet" href="/css/main.css?v=2.12.2"><script src="/js/xpx-features.js?v=2.12.2"></script></head>`, 1)
	idx = strings.Replace(idx, "</body>", `<script src="/js/main.js?v=2.12.1"></script></body>`, 1)
	indexHTML := []byte(idx)

	// Public
	r.Get("/version", Version)
	r.Get("/health/live", HealthLive)
	r.Get("/health/ready", HealthReady)
	r.Get("/api-docs", SwaggerUI)
	r.Get("/api-docs/swagger.json", SwaggerJSON)
	r.Post("/login", rateLimitMiddleware(loginLimiter)(http.HandlerFunc(Login)).ServeHTTP)
	r.Post("/initialize", Initialize)
	r.Get("/oauth/{provider}/login", OAuthLogin)
	r.Get("/oauth/{provider}/callback", OAuthCallback)
	r.Post("/api/webhooks/{token}", WebhookTrigger)
	r.Post("/api/webhooks/git/{id}", GitWebhookHandler)
	// SSE must be public — EventSource API cannot send Authorization headers.
	// The legacy frontend authenticates via SLT query param.
	r.Get("/events", SSEHandler)
	r.Post("/events", EventPush)
	r.Get("/slt", func(w http.ResponseWriter, r *http.Request) {
		json200(w, map[string]string{"slt": "go-backend-slt"})
	})

	// Authenticated
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)

		// #71: SSE and agent events now require auth
		// NOTE: moved back to public routes — EventSource cannot send headers

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
		r.Delete("/api/services/{id}", auth.WriteOnly(http.HandlerFunc(ServiceDelete)).ServeHTTP)
		r.Post("/api/services", auth.WriteOnly(http.HandlerFunc(ServiceCreate)).ServeHTTP)
		r.Post("/api/services/{id}/update", auth.WriteOnly(http.HandlerFunc(ServiceUpdate)).ServeHTTP)
		r.Post("/api/services/{id}/redeploy", auth.WriteOnly(http.HandlerFunc(ServiceRedeploy)).ServeHTTP)
		r.Post("/api/services/{id}/rollback", auth.WriteOnly(http.HandlerFunc(ServiceRollback)).ServeHTTP)
		r.Get("/api/tasks", TaskList)
		r.Get("/api/tasks/{id}", TaskInfo)
		r.Get("/api/tasks/{id}/ts", TaskTimeseries)
		r.Get("/api/networks", NetworkList)
		r.Get("/api/networks/{id}", NetworkInfo)
		r.Get("/api/networks/{id}/services", NetworkServices)
		r.Delete("/api/networks/{id}", auth.WriteOnly(http.HandlerFunc(NetworkDelete)).ServeHTTP)
		r.Post("/api/networks", auth.WriteOnly(http.HandlerFunc(NetworkCreate)).ServeHTTP)
		r.Get("/api/volumes", VolumeList)
		r.Get("/api/volumes/{id}", VolumeInfo)
		r.Get("/api/volumes/{id}/services", VolumeServices)
		r.Post("/api/volumes", auth.WriteOnly(http.HandlerFunc(VolumeCreate)).ServeHTTP)
		r.Get("/api/secrets", SecretList)
		r.Get("/api/secrets/{id}", SecretInfo)
		r.Get("/api/secrets/{id}/services", SecretServices)
		r.Delete("/api/secrets/{id}", auth.WriteOnly(http.HandlerFunc(SecretDelete)).ServeHTTP)
		r.Post("/api/secrets", auth.WriteOnly(http.HandlerFunc(SecretCreate)).ServeHTTP)
		r.Put("/api/secrets/{id}", auth.WriteOnly(http.HandlerFunc(SecretUpdate)).ServeHTTP)
		r.Get("/api/configs", ConfigList)
		r.Get("/api/configs/{id}", ConfigInfo)
		r.Get("/api/configs/{id}/services", ConfigServices)
		r.Delete("/api/configs/{id}", auth.WriteOnly(http.HandlerFunc(ConfigDelete)).ServeHTTP)
		r.Post("/api/configs", auth.WriteOnly(http.HandlerFunc(ConfigCreate)).ServeHTTP)
		r.Put("/api/configs/{id}", auth.WriteOnly(http.HandlerFunc(ConfigUpdate)).ServeHTTP)
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
		r.Post("/api/stacks/git", auth.WriteOnly(http.HandlerFunc(GitDeploy)).ServeHTTP)
		r.Post("/api/stacks", auth.WriteOnly(http.HandlerFunc(StackCreate)).ServeHTTP)
		r.Put("/api/stacks/{name}", auth.WriteOnly(auth.TeamPermissionCheck(http.HandlerFunc(StackUpdate))).ServeHTTP)
		r.Delete("/api/stacks/{name}", auth.WriteOnly(auth.TeamPermissionCheck(http.HandlerFunc(StackDelete))).ServeHTTP)
		r.Post("/api/stacks/{name}/redeploy", auth.WriteOnly(auth.TeamPermissionCheck(http.HandlerFunc(StackRedeploy))).ServeHTTP)
		r.Post("/api/stacks/{name}/rollback", auth.WriteOnly(auth.TeamPermissionCheck(http.HandlerFunc(StackRollback))).ServeHTTP)
		r.Post("/api/services/{id}/stop", auth.WriteOnly(http.HandlerFunc(ServiceStop)).ServeHTTP)
		r.Put("/api/nodes/{id}", auth.WriteOnly(http.HandlerFunc(NodeEdit)).ServeHTTP)
		r.Post("/api/stacks/{name}/activate", auth.WriteOnly(auth.TeamPermissionCheck(http.HandlerFunc(StackActivate))).ServeHTTP)
		r.Post("/api/stacks/{name}/deactivate", auth.WriteOnly(auth.TeamPermissionCheck(http.HandlerFunc(StackDeactivate))).ServeHTTP)
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

		r.Get("/api/gitops", GitStackList)
		r.Post("/api/gitops", GitStackCreate)
		r.Get("/api/gitops/{id}", GitStackGet)
		r.Put("/api/gitops/{id}", GitStackUpdate)
		r.Delete("/api/gitops/{id}", GitStackDelete)
		r.Post("/api/gitops/{id}/sync", GitStackSync)

		// #93: TOTP
		r.Post("/api/totp/setup", TOTPSetup)
		r.Post("/api/totp/disable", TOTPDisable)

		r.Group(func(r chi.Router) {
			r.Use(auth.AdminOnly)
			r.Get("/api/audit", AuditList)
			r.Get("/api/users", UserList)
			r.Post("/api/users", UserCreate)
			r.Put("/api/users/{id}", UserUpdate)
			r.Delete("/api/users/{id}", UserDelete)
			r.Get("/api/backup", BackupHandler)
			r.Post("/api/restore", RestoreHandler)
			r.Post("/api/services/check-updates", CheckImageUpdates)
			r.Get("/api/services/update-status", GetImageUpdateStatus)
			r.Post("/api/system/prune", PruneSystem)
			r.Post("/api/backup/s3", BackupToS3)
			r.Get("/api/backup/s3", ListS3Backups)
			r.Post("/api/restore/s3", RestoreFromS3)
			// #89: Bulk stack import
			r.Post("/api/stacks/import", StackImport)
			// #99: Team permissions
			r.Get("/api/teams", TeamPermissionList)
			r.Post("/api/teams", TeamPermissionCreate)
			r.Delete("/api/teams/{id}", TeamPermissionDelete)
			// #100: Multi-cluster
			r.Get("/api/clusters", ClusterList)
			r.Post("/api/clusters", ClusterCreate)
			r.Delete("/api/clusters/{id}", ClusterDelete)
			r.Post("/api/clusters/{id}/activate", ClusterActivate)
		})
	})

	// Static files + SPA fallback (#97: serve index for unknown paths)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[1:]
		if path == "" {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(indexHTML)
			return
		}
		f, err := staticFS.Open(path)
		if err != nil {
			// SPA fallback
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(indexHTML)
			return
		}
		defer f.Close()
		stat, _ := f.Stat()
		if stat.IsDir() {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "no-store")
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
