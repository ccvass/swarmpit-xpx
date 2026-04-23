package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/store"
	"github.com/go-chi/chi/v5"
)

var gitWorkDir string

func initGitWorkDir() {
	gitWorkDir = filepath.Join(os.TempDir(), "swarmpit-gitops")
	os.MkdirAll(gitWorkDir, 0755)
}

// ─── API Handlers ───

func GitStackList(w http.ResponseWriter, r *http.Request) {
	json200(w, store.ListGitStacks())
}

func GitStackGet(w http.ResponseWriter, r *http.Request) {
	gs, err := store.GetGitStack(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 404, "git stack not found")
		return
	}
	gs.Credentials = "" // never expose
	json200(w, gs)
}

func GitStackCreate(w http.ResponseWriter, r *http.Request) {
	var gs store.GitStack
	json.NewDecoder(r.Body).Decode(&gs)
	if gs.RepoURL == "" || gs.StackName == "" {
		jsonErr(w, 400, "repoUrl and stackName required")
		return
	}
	gs.Enabled = true
	created, err := store.CreateGitStack(gs)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	// Initial clone + deploy
	go func() {
		if err := syncGitStack(created); err != nil {
			slog.Error("gitops initial sync failed", "stack", created.StackName, "err", err)
		}
	}()

	store.RecordAudit(authUser(r), "create", "git-stack", gs.StackName)
	json200(w, created)
}

func GitStackUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var gs store.GitStack
	json.NewDecoder(r.Body).Decode(&gs)
	if err := store.UpdateGitStack(id, gs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	store.RecordAudit(authUser(r), "update", "git-stack", gs.StackName)
	json200(w, map[string]string{"status": "updated"})
}

func GitStackDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gs, err := store.GetGitStack(id)
	if err != nil {
		jsonErr(w, 404, "not found")
		return
	}
	// Cleanup cloned repo
	os.RemoveAll(repoDir(gs))
	store.DeleteGitStack(id)
	store.RecordAudit(authUser(r), "delete", "git-stack", gs.StackName)
	json200(w, map[string]string{"status": "deleted"})
}

func GitStackSync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gs, err := store.GetGitStack(id)
	if err != nil {
		jsonErr(w, 404, "not found")
		return
	}
	if err := syncGitStack(gs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	store.RecordAudit(authUser(r), "sync", "git-stack", gs.StackName)
	updated, _ := store.GetGitStack(id)
	json200(w, updated)
}

func GitWebhookHandler(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	gs, err := store.GetGitStack(stackID)
	if err != nil {
		// Try by stack name
		gs, err = store.GetGitStackByName(stackID)
		if err != nil {
			jsonErr(w, 404, "git stack not found")
			return
		}
	}
	if !gs.Enabled {
		json200(w, map[string]string{"status": "disabled"})
		return
	}
	go func() {
		if err := syncGitStack(gs); err != nil {
			slog.Error("gitops webhook sync failed", "stack", gs.StackName, "err", err)
		}
	}()
	json200(w, map[string]string{"status": "sync triggered"})
}

// ─── Git Operations ───

func repoDir(gs store.GitStack) string {
	return filepath.Join(gitWorkDir, gs.ID)
}

func gitCloneOrPull(gs store.GitStack) error {
	dir := repoDir(gs)
	repoURL := injectCredentials(gs.RepoURL, gs.Credentials)

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Pull
		cmd := exec.Command("git", "-C", dir, "fetch", "--depth=1", "origin", gs.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git fetch: %s", string(out))
		}
		cmd = exec.Command("git", "-C", dir, "reset", "--hard", "origin/"+gs.Branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git reset: %s", string(out))
		}
		return nil
	}

	// Clone
	os.MkdirAll(dir, 0755)
	cmd := exec.Command("git", "clone", "--depth=1", "--branch", gs.Branch, "--single-branch", repoURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("git clone: %s", string(out))
	}
	return nil
}

func injectCredentials(repoURL, creds string) string {
	if creds == "" {
		return repoURL
	}
	// For HTTPS URLs, inject token: https://token@github.com/...
	if strings.HasPrefix(repoURL, "https://") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return repoURL
		}
		u.User = url.UserPassword("x-access-token", creds)
		return u.String()
	}
	return repoURL
}

func readComposeFile(gs store.GitStack) (string, error) {
	path := filepath.Join(repoDir(gs), gs.ComposePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("compose file not found: %s", gs.ComposePath)
	}
	return string(data), nil
}

func fileHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}

// ─── Sync Logic ───

func syncGitStack(gs store.GitStack) error {
	now := time.Now().UTC().Format(time.RFC3339)

	if err := gitCloneOrPull(gs); err != nil {
		store.UpdateGitStackSync(gs.ID, gs.LastHash, now, err.Error())
		return err
	}

	spec, err := readComposeFile(gs)
	if err != nil {
		store.UpdateGitStackSync(gs.ID, gs.LastHash, now, err.Error())
		return err
	}

	hash := fileHash(spec)
	if hash == gs.LastHash {
		store.UpdateGitStackSync(gs.ID, hash, now, "")
		slog.Debug("gitops no changes", "stack", gs.StackName, "hash", hash)
		return nil
	}

	// Deploy
	out, err := deployStack(gs.StackName, spec)
	if err != nil {
		errMsg := fmt.Sprintf("deploy failed: %s", out)
		store.UpdateGitStackSync(gs.ID, gs.LastHash, now, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	store.UpdateGitStackSync(gs.ID, hash, now, "")
	store.RecordAudit("gitops", "deploy", "git-stack", gs.StackName)
	slog.Info("gitops deployed", "stack", gs.StackName, "hash", hash)
	return nil
}

// ─── Scheduler ───

func StartGitOpsScheduler() {
	initGitWorkDir()
	go func() {
		time.Sleep(60 * time.Second) // initial delay
		for {
			stacks := store.ListGitStacks()
			for _, gs := range stacks {
				if !gs.Enabled || gs.SyncInterval <= 0 {
					continue
				}
				// Check if enough time has passed since last sync
				if gs.LastSync != "" {
					last, err := time.Parse(time.RFC3339, gs.LastSync)
					if err == nil && time.Since(last) < time.Duration(gs.SyncInterval)*time.Second {
						continue
					}
				}
				go func(s store.GitStack) {
					if err := syncGitStack(s); err != nil {
						slog.Error("gitops scheduled sync failed", "stack", s.StackName, "err", err)
					}
				}(gs)
			}
			time.Sleep(30 * time.Second) // check loop interval
		}
	}()
	slog.Info("gitops scheduler started")
}
