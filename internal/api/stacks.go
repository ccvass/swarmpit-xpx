package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// StackList returns all stacks (grouped services by com.docker.stack.namespace label)
func StackList(w http.ResponseWriter, r *http.Request) {
	svcs, err := docker.Services()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	stacks := make(map[string]map[string]any)
	for _, svc := range svcs {
		ns := svc.Spec.Labels["com.docker.stack.namespace"]
		if ns == "" {
			continue
		}
		if _, ok := stacks[ns]; !ok {
			stacks[ns] = map[string]any{"stackName": ns, "serviceCount": 0}
		}
		stacks[ns]["serviceCount"] = stacks[ns]["serviceCount"].(int) + 1
	}
	result := make([]map[string]any, 0, len(stacks))
	for _, s := range stacks {
		result = append(result, s)
	}
	json200(w, result)
}

// StackInfo returns stack details
func StackInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, err := docker.Services()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	var stackServices []any
	for _, svc := range svcs {
		if svc.Spec.Labels["com.docker.stack.namespace"] == name {
			stackServices = append(stackServices, svc)
		}
	}
	if stackServices == nil {
		stackServices = []any{}
	}
	json200(w, map[string]any{"stackName": name, "services": stackServices})
}

// GitDeploy deploys a stack from a git repository
func GitDeploy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RepoURL     string `json:"repo-url"`
		Branch      string `json:"branch"`
		ComposePath string `json:"compose-path"`
		StackName   string `json:"stack-name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if body.RepoURL == "" || body.StackName == "" {
		jsonErr(w, 400, "repo-url and stack-name required")
		return
	}
	if body.Branch == "" {
		body.Branch = "main"
	}
	if body.ComposePath == "" {
		body.ComposePath = "docker-compose.yml"
	}

	tmpDir := filepath.Join(os.TempDir(), "swarmpit-git-"+uuid.NewString())
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "clone", "--depth", "1", "-b", body.Branch, body.RepoURL, tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("git clone failed", "err", err, "output", string(out))
		jsonErr(w, 400, fmt.Sprintf("git clone failed: %s", string(out)))
		return
	}

	composePath := filepath.Join(tmpDir, body.ComposePath)
	if _, err := os.Stat(composePath); err != nil {
		jsonErr(w, 400, "compose file not found: "+body.ComposePath)
		return
	}

	deploy := exec.Command("docker", "stack", "deploy", "--with-registry-auth", "--compose-file", composePath, body.StackName)
	if out, err := deploy.CombinedOutput(); err != nil {
		slog.Error("stack deploy failed", "err", err, "output", string(out))
		jsonErr(w, 400, fmt.Sprintf("deploy failed: %s", string(out)))
		return
	}

	slog.Info("stack deployed from git", "stack", body.StackName, "repo", body.RepoURL)
	json200(w, map[string]string{"stack": body.StackName, "source": body.RepoURL, "branch": body.Branch})
}
