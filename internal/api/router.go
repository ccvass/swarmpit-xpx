package api

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/ccvass/swarmpit-xpx/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(staticFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Read index.html once for SPA fallback
	indexHTML, _ := fs.ReadFile(staticFS, "index.html")

	// Public
	r.Get("/version", Version)
	r.Get("/health/live", HealthLive)
	r.Get("/health/ready", HealthReady)
	r.Post("/login", Login)
	r.Post("/initialize", Initialize)
	r.Post("/api/webhooks/{token}", WebhookTrigger)
	r.Get("/events", SSEHandler)
	r.Post("/events", EventPush)

	// Authenticated
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)

		r.Get("/api/nodes", NodeList)
		r.Get("/api/nodes/{id}", NodeInfo)
		r.Get("/api/services", ServiceList)
		r.Get("/api/services/{id}", ServiceInfo)
		r.Get("/api/tasks", TaskList)
		r.Get("/api/networks", NetworkList)
		r.Get("/api/secrets", SecretList)
		r.Get("/api/configs", ConfigList)
		r.Get("/api/stacks", StackList)
		r.Get("/api/stacks/{name}", StackInfo)
		r.Post("/api/stacks/git", GitDeploy)
		r.Get("/exec/{id}", ExecHandler)

		r.Group(func(r chi.Router) {
			r.Use(auth.AdminOnly)
			r.Get("/api/audit", AuditList)
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
