package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ccvass/swarmpit-xpx/internal/api"
	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
)

func main() {
	log.Println("Swarmpit XPX starting...")

	// Database
	dbPath := envOr("SWARMPIT_DB_PATH", "./data")
	if err := store.Init(dbPath); err != nil {
		log.Fatal("Database init failed:", err)
	}
	log.Println("SQLite ready at", dbPath)

	// Docker
	if err := docker.Init(); err != nil {
		log.Fatal("Docker init failed:", err)
	}
	ping, _ := docker.Ping()
	log.Println("Docker API:", ping.APIVersion)

	// Static files
	publicDir := envOr("SWARMPIT_PUBLIC_DIR", "resources/public")
	staticFS := os.DirFS(publicDir)

	// Server
	port := envOr("PORT", "8080")
	router := api.NewRouter(staticFS)
	log.Println("Swarmpit running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
