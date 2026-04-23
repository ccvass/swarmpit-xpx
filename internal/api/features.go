package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
	"github.com/docker/docker/api/types/filters"
	"github.com/go-chi/chi/v5"

	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// ─── #66 Image Update Notifications ───

var imageUpdates = struct {
	sync.RWMutex
	data map[string]bool // serviceID → has update
}{data: map[string]bool{}}

func CheckImageUpdates(w http.ResponseWriter, r *http.Request) {
	services, err := docker.Services()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	results := []map[string]any{}
	for _, svc := range services {
		img := svc.Spec.TaskTemplate.ContainerSpec.Image
		hasUpdate := checkRegistryDigest(img)
		imageUpdates.Lock()
		imageUpdates.data[svc.ID] = hasUpdate
		imageUpdates.Unlock()
		if hasUpdate {
			results = append(results, map[string]any{
				"serviceId":   svc.ID,
				"serviceName": svc.Spec.Name,
				"image":       img,
				"hasUpdate":   true,
			})
		}
	}
	store.RecordAudit(authUser(r), "check", "image-updates", fmt.Sprintf("%d updates found", len(results)))
	json200(w, results)
}

func GetImageUpdateStatus(w http.ResponseWriter, r *http.Request) {
	imageUpdates.RLock()
	defer imageUpdates.RUnlock()
	result := map[string]bool{}
	for k, v := range imageUpdates.data {
		if v {
			result[k] = true
		}
	}
	json200(w, result)
}

func checkRegistryDigest(image string) bool {
	// image format: registry/repo:tag@sha256:digest or repo:tag
	// If pinned to digest, skip
	if strings.Contains(image, "@sha256:") {
		parts := strings.SplitN(image, "@", 2)
		currentDigest := parts[1]
		ref := parts[0]
		remoteDigest := fetchRemoteDigest(ref)
		if remoteDigest == "" {
			return false
		}
		return remoteDigest != currentDigest
	}
	return false
}

func fetchRemoteDigest(ref string) string {
	// Parse image reference
	registry, repo, tag := parseImageRef(ref)
	if tag == "" {
		tag = "latest"
	}

	var manifestURL string
	var req *http.Request

	if registry == "docker.io" || registry == "" {
		// DockerHub: get token first
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
		tokenURL := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
		resp, err := httpClient.Get(tokenURL)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		var tok struct {
			Token string `json:"token"`
		}
		json.NewDecoder(resp.Body).Decode(&tok)
		manifestURL = fmt.Sprintf("https://registry-1.docker.io/v2/%s/manifests/%s", repo, tag)
		req, _ = http.NewRequest("HEAD", manifestURL, nil)
		req.Header.Set("Authorization", "Bearer "+tok.Token)
	} else if registry == "ghcr.io" {
		ghToken := os.Getenv("GHCR_TOKEN")
		if ghToken == "" {
			return ""
		}
		manifestURL = fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", repo, tag)
		req, _ = http.NewRequest("HEAD", manifestURL, nil)
		req.Header.Set("Authorization", "Bearer "+ghToken)
	} else {
		manifestURL = fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
		req, _ = http.NewRequest("HEAD", manifestURL, nil)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()
	return resp.Header.Get("Docker-Content-Digest")
}

func parseImageRef(ref string) (registry, repo, tag string) {
	// Split tag
	tagIdx := strings.LastIndex(ref, ":")
	if tagIdx > 0 && !strings.Contains(ref[tagIdx:], "/") {
		tag = ref[tagIdx+1:]
		ref = ref[:tagIdx]
	}
	// Split registry
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repo = parts[1]
	} else {
		registry = "docker.io"
		repo = ref
	}
	return
}

func StartImageUpdateChecker() {
	interval := 1 * time.Hour
	if v := os.Getenv("IMAGE_CHECK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}
	go func() {
		time.Sleep(30 * time.Second) // initial delay
		for {
			runImageCheck()
			time.Sleep(interval)
		}
	}()
	slog.Info("image update checker started", "interval", interval)
}

func runImageCheck() {
	services, err := docker.Services()
	if err != nil {
		return
	}
	for _, svc := range services {
		img := svc.Spec.TaskTemplate.ContainerSpec.Image
		hasUpdate := checkRegistryDigest(img)
		imageUpdates.Lock()
		imageUpdates.data[svc.ID] = hasUpdate
		imageUpdates.Unlock()
	}
}

// ─── #67 Cleanup Unused Resources ───

func PruneSystem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Images   bool `json:"images"`
		Volumes  bool `json:"volumes"`
		Networks bool `json:"networks"`
		DryRun   bool `json:"dryRun"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	// Default: prune all if nothing specified
	if !req.Images && !req.Volumes && !req.Networks {
		req.Images = true
		req.Volumes = true
		req.Networks = true
	}

	result := map[string]any{}
	cli := docker.Client()
	ctx := context.Background()

	if req.Images {
		if req.DryRun {
			images, _ := cli.ImageList(ctx, docker.ImageListOpts(true))
			space := int64(0)
			count := 0
			for _, img := range images {
				if len(img.RepoTags) == 0 || img.RepoTags[0] == "<none>:<none>" {
					space += img.Size
					count++
				}
			}
			result["images"] = map[string]any{"count": count, "spaceReclaimed": space}
		} else {
			report, _ := cli.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
			result["images"] = map[string]any{
				"count":          len(report.ImagesDeleted),
				"spaceReclaimed": report.SpaceReclaimed,
			}
		}
	}

	if req.Volumes {
		if req.DryRun {
			vols, _ := docker.Volumes()
			result["volumes"] = map[string]any{"count": len(vols.Volumes)}
		} else {
			report, _ := cli.VolumesPrune(ctx, filters.NewArgs())
			result["volumes"] = map[string]any{
				"count":          len(report.VolumesDeleted),
				"spaceReclaimed": report.SpaceReclaimed,
			}
		}
	}

	if req.Networks {
		if req.DryRun {
			nets, _ := docker.Networks()
			unused := 0
			for _, n := range nets {
				if n.Driver != "null" && !n.Ingress && len(n.Containers) == 0 {
					unused++
				}
			}
			result["networks"] = map[string]any{"count": unused}
		} else {
			report, _ := cli.NetworksPrune(ctx, filters.NewArgs())
			result["networks"] = map[string]any{"count": len(report.NetworksDeleted)}
		}
	}

	result["dryRun"] = req.DryRun
	if !req.DryRun {
		store.RecordAudit(authUser(r), "prune", "system", fmt.Sprintf("images=%v volumes=%v networks=%v", req.Images, req.Volumes, req.Networks))
	}
	json200(w, result)
}

// ─── #68 S3 Scheduled Backups ───

func BackupToS3(w http.ResponseWriter, r *http.Request) {
	cfg := s3Config()
	if cfg.bucket == "" {
		jsonErr(w, 400, "S3 backup not configured — set BACKUP_S3_BUCKET")
		return
	}
	key, err := uploadBackupToS3(cfg)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	store.RecordAudit(authUser(r), "backup", "s3", key)
	json200(w, map[string]string{"key": key, "bucket": cfg.bucket})
}

func ListS3Backups(w http.ResponseWriter, r *http.Request) {
	cfg := s3Config()
	if cfg.bucket == "" {
		jsonErr(w, 400, "S3 backup not configured")
		return
	}
	backups, err := listS3Backups(cfg)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, backups)
}

func RestoreFromS3(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Key == "" {
		jsonErr(w, 400, "key required")
		return
	}
	cfg := s3Config()
	if cfg.bucket == "" {
		jsonErr(w, 400, "S3 backup not configured")
		return
	}
	if err := restoreFromS3(cfg, req.Key); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	store.RecordAudit(authUser(r), "restore", "s3", req.Key)
	json200(w, map[string]string{"status": "restored", "key": req.Key})
}

type s3Cfg struct {
	endpoint  string
	bucket    string
	accessKey string
	secretKey string
	region    string
	retention int // days
}

func s3Config() s3Cfg {
	ret := 30
	if v := os.Getenv("BACKUP_RETENTION_DAYS"); v != "" {
		fmt.Sscanf(v, "%d", &ret)
	}
	return s3Cfg{
		endpoint:  os.Getenv("BACKUP_S3_ENDPOINT"),
		bucket:    os.Getenv("BACKUP_S3_BUCKET"),
		accessKey: os.Getenv("BACKUP_S3_KEY"),
		secretKey: os.Getenv("BACKUP_S3_SECRET"),
		region:    envOrDefault("BACKUP_S3_REGION", "us-east-1"),
		retention: ret,
	}
}

func envOrDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func uploadBackupToS3(cfg s3Cfg) (string, error) {
	dbPath := filepath.Join(store.DBDir(), "swarmpit.db")
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return "", fmt.Errorf("read db: %w", err)
	}
	key := fmt.Sprintf("swarmpit-backup-%s.db", time.Now().UTC().Format("20060102-150405"))
	if err := s3Put(cfg, key, data); err != nil {
		return "", err
	}
	slog.Info("s3 backup uploaded", "key", key, "size", len(data))
	return key, nil
}

func restoreFromS3(cfg s3Cfg, key string) error {
	data, err := s3Get(cfg, key)
	if err != nil {
		return err
	}
	dbPath := filepath.Join(store.DBDir(), "swarmpit.db")
	return os.WriteFile(dbPath, data, 0644)
}

func listS3Backups(cfg s3Cfg) ([]map[string]any, error) {
	body, err := s3ListObjects(cfg, "swarmpit-backup-")
	if err != nil {
		return nil, err
	}
	// Parse simple XML response
	results := []map[string]any{}
	for _, entry := range extractXMLTags(body, "Key") {
		results = append(results, map[string]any{"key": entry})
	}
	return results, nil
}

func StartBackupScheduler() {
	schedule := os.Getenv("BACKUP_SCHEDULE")
	if schedule == "" {
		return
	}
	cfg := s3Config()
	if cfg.bucket == "" {
		return
	}
	// Simple interval-based scheduler (e.g. "6h", "24h")
	interval, err := time.ParseDuration(schedule)
	if err != nil {
		slog.Warn("invalid BACKUP_SCHEDULE", "value", schedule)
		return
	}
	go func() {
		for {
			time.Sleep(interval)
			key, err := uploadBackupToS3(cfg)
			if err != nil {
				slog.Error("scheduled backup failed", "err", err)
				continue
			}
			slog.Info("scheduled backup completed", "key", key)
			pruneOldBackups(cfg)
		}
	}()
	slog.Info("s3 backup scheduler started", "interval", interval, "bucket", cfg.bucket)
}

func pruneOldBackups(cfg s3Cfg) {
	backups, err := listS3Backups(cfg)
	if err != nil || len(backups) <= cfg.retention {
		return
	}
	// Sort by key (contains timestamp) and delete oldest
	keys := make([]string, len(backups))
	for i, b := range backups {
		keys[i] = b["key"].(string)
	}
	sort.Strings(keys)
	for _, k := range keys[:len(keys)-cfg.retention] {
		s3Delete(cfg, k)
		slog.Info("pruned old backup", "key", k)
	}
}

// ─── Minimal S3 client (no SDK dependency) ───

func s3Put(cfg s3Cfg, key string, data []byte) error {
	endpoint := cfg.endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.region)
	}
	url := fmt.Sprintf("%s/%s/%s", endpoint, cfg.bucket, key)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	signS3(req, cfg, data)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3 put %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func s3Get(cfg s3Cfg, key string) ([]byte, error) {
	endpoint := cfg.endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.region)
	}
	url := fmt.Sprintf("%s/%s/%s", endpoint, cfg.bucket, key)
	req, _ := http.NewRequest("GET", url, nil)
	signS3(req, cfg, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("s3 get %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func s3Delete(cfg s3Cfg, key string) {
	endpoint := cfg.endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.region)
	}
	url := fmt.Sprintf("%s/%s/%s", endpoint, cfg.bucket, key)
	req, _ := http.NewRequest("DELETE", url, nil)
	signS3(req, cfg, nil)
	resp, err := httpClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func s3ListObjects(cfg s3Cfg, prefix string) (string, error) {
	endpoint := cfg.endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.region)
	}
	url := fmt.Sprintf("%s/%s?prefix=%s", endpoint, cfg.bucket, prefix)
	req, _ := http.NewRequest("GET", url, nil)
	signS3(req, cfg, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func signS3(req *http.Request, cfg s3Cfg, payload []byte) {
	if cfg.accessKey == "" {
		return
	}
	now := time.Now().UTC()
	date := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", sha256Hex(payload))

	scope := fmt.Sprintf("%s/%s/s3/aws4_request", date, cfg.region)
	canonHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", req.URL.Host, sha256Hex(payload), amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonReq := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s", req.Method, req.URL.Path, req.URL.RawQuery, canonHeaders, signedHeaders, sha256Hex(payload))
	strToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, scope, sha256Hex([]byte(canonReq)))

	kDate := hmacSHA256([]byte("AWS4"+cfg.secretKey), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(cfg.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(strToSign)))

	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", cfg.accessKey, scope, signedHeaders, sig))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func extractXMLTags(xml, tag string) []string {
	var results []string
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	for {
		i := strings.Index(xml, open)
		if i < 0 {
			break
		}
		xml = xml[i+len(open):]
		j := strings.Index(xml, close)
		if j < 0 {
			break
		}
		results = append(results, xml[:j])
		xml = xml[j+len(close):]
	}
	return results
}

// ─── Helpers ───

func authUser(r *http.Request) string {
	if u := chi.URLParam(r, "username"); u != "" {
		return u
	}
	if u, ok := r.Context().Value("username").(string); ok {
		return u
	}
	return "system"
}
