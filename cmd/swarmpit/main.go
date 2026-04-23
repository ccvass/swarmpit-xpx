package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/ccvass/swarmpit-xpx/internal/api"
	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
)

func main() {
	slog.Info("swarmpit starting")

	dbPath := envOr("SWARMPIT_DB_PATH", "./data")
	if err := store.Init(dbPath); err != nil {
		slog.Error("database init failed", "err", err)
		os.Exit(1)
	}
	slog.Info("sqlite ready", "path", dbPath)

	if err := docker.Init(); err != nil {
		slog.Error("docker init failed", "err", err)
		os.Exit(1)
	}
	ping, _ := docker.Ping()
	slog.Info("docker connected", "api", ping.APIVersion)

	api.InitTimeseries()
	api.StartAlertChecker()
	api.StartImageUpdateChecker()
	api.StartBackupScheduler()
	api.StartGitOpsScheduler()

	publicDir := envOr("SWARMPIT_PUBLIC_DIR", "resources/public")
	port := envOr("PORT", "8080")
	router := api.NewRouter(os.DirFS(publicDir))

	slog.Info("swarmpit running", "port", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
