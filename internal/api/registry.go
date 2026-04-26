package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/store"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func fetchAuthenticatedTags(host, repo string) ([]map[string]any, error) {
	reg, err := store.FindRegistryByHost(host)
	if err != nil {
		return nil, err
	}
	username, _ := reg["username"].(string)
	password, _ := reg["password"].(string)
	if password == "" {
		password, _ = reg["token"].(string)
	}

	// GHCR token-based auth
	if host == "ghcr.io" {
		return fetchTagsWithBearer(
			fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull&service=ghcr.io", repo),
			fmt.Sprintf("https://ghcr.io/v2/%s/tags/list", repo),
			username, password,
		)
	}
	// GitLab registry (#85)
	if strings.Contains(host, "gitlab") || strings.Contains(host, "registry.gitlab") {
		authURL := fmt.Sprintf("https://%s/jwt/auth?service=container_registry&scope=repository:%s:pull", host, repo)
		return fetchTagsWithBearer(authURL, fmt.Sprintf("https://%s/v2/%s/tags/list", host, repo), username, password)
	}
	// DockerHub (#85)
	if host == "index.docker.io" || host == "registry-1.docker.io" || host == "docker.io" {
		return fetchTagsWithBearer(
			fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo),
			fmt.Sprintf("https://registry-1.docker.io/v2/%s/tags/list", repo),
			username, password,
		)
	}
	// ECR — accept pre-generated token (#85)
	if strings.Contains(host, ".dkr.ecr.") && strings.Contains(host, ".amazonaws.com") {
		if password != "" {
			req, _ := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/%s/tags/list", host, repo), nil)
			req.SetBasicAuth("AWS", password)
			return doTagsRequest(req)
		}
	}
	// Generic v2 with basic auth, fallback to token auth via WWW-Authenticate
	u := fmt.Sprintf("https://%s/v2/%s/tags/list", host, repo)
	req, _ := http.NewRequest("GET", u, nil)
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}
	tags, err := doTagsRequest(req)
	if err != nil && username != "" && password != "" {
		// Basic auth failed — try token-based auth by probing /v2/ for WWW-Authenticate
		probeReq, _ := http.NewRequest("GET", fmt.Sprintf("https://%s/v2/", host), nil)
		probeResp, probeErr := httpClient.Do(probeReq)
		if probeErr == nil {
			probeResp.Body.Close()
			wwwAuth := probeResp.Header.Get("Www-Authenticate")
			if strings.Contains(wwwAuth, "Bearer") {
				realm, service := parseWWWAuthenticate(wwwAuth)
				if realm != "" {
					authURL := fmt.Sprintf("%s?service=%s&scope=repository:%s:pull", realm, service, repo)
					return fetchTagsWithBearer(authURL, u, username, password)
				}
			}
		}
	}
	return tags, err
}

func parseWWWAuthenticate(header string) (realm, service string) {
	// Parse: Bearer realm="https://...",service="..."
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "Bearer ") {
			part = strings.TrimPrefix(part, "Bearer ")
		}
		if strings.HasPrefix(part, "realm=") {
			realm = strings.Trim(strings.TrimPrefix(part, "realm="), "\"")
		}
		if strings.HasPrefix(part, "service=") {
			service = strings.Trim(strings.TrimPrefix(part, "service="), "\"")
		}
	}
	return
}

// fetchTagsWithBearer gets a bearer token then fetches tags
func fetchTagsWithBearer(tokenURL, tagsURL, username, password string) ([]map[string]any, error) {
	req, _ := http.NewRequest("GET", tokenURL, nil)
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	} else if password != "" {
		req.SetBasicAuth("token", password)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tokenResp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)
	tok := tokenResp.Token
	if tok == "" {
		tok = tokenResp.AccessToken
	}
	if tok == "" {
		return nil, fmt.Errorf("auth failed: no token returned")
	}
	tagsReq, _ := http.NewRequest("GET", tagsURL, nil)
	tagsReq.Header.Set("Authorization", "Bearer "+tok)
	return doTagsRequest(tagsReq)
}

// doTagsRequest executes a tags/list request and returns parsed results
func doTagsRequest(req *http.Request) ([]map[string]any, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}
	var data struct {
		Tags []string `json:"tags"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	result := make([]map[string]any, len(data.Tags))
	for i, t := range data.Tags {
		result[i] = map[string]any{"name": t}
	}
	return result, nil
}

func searchDockerHub(query string) ([]map[string]any, error) {
	u := fmt.Sprintf("https://hub.docker.com/v2/search/repositories/?query=%s&page_size=20", url.QueryEscape(query))
	resp, err := httpClient.Get(u)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var data struct {
		Results []struct {
			Name      string `json:"repo_name"`
			Desc      string `json:"short_description"`
			Stars     int    `json:"star_count"`
			Official  bool   `json:"is_official"`
			Automated bool   `json:"is_automated"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil { return nil, err }
	result := make([]map[string]any, len(data.Results))
	for i, r := range data.Results {
		result[i] = map[string]any{
			"name": r.Name, "description": r.Desc,
			"stars": r.Stars, "official": r.Official, "automated": r.Automated,
		}
	}
	return result, nil
}

func fetchDockerHubTags(repo string) ([]map[string]any, error) {
	u := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags/?page_size=50", repo)
	resp, err := httpClient.Get(u)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var data struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil { return nil, err }
	result := make([]map[string]any, len(data.Results))
	for i, r := range data.Results {
		result[i] = map[string]any{"name": r.Name}
	}
	return result, nil
}

func listRegistryRepos(reg map[string]any) ([]map[string]any, error) {
	// For v2 registries, call the catalog API
	regURL, _ := reg["url"].(string)
	if regURL == "" { return []map[string]any{}, nil }
	u := fmt.Sprintf("%s/v2/_catalog", regURL)
	resp, err := httpClient.Get(u)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var data struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil { return nil, err }
	result := make([]map[string]any, len(data.Repositories))
	for i, r := range data.Repositories {
		result[i] = map[string]any{"name": r}
	}
	return result, nil
}

func fetchRegistryV2Tags(fullRepo string) ([]map[string]any, error) {
	// fullRepo format: registry.example.com/org/repo
	parts := strings.SplitN(fullRepo, "/", 2)
	if len(parts) < 2 { return nil, fmt.Errorf("invalid repo format") }
	registry, repo := parts[0], parts[1]
	u := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repo)
	resp, err := httpClient.Get(u)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("registry returned %d", resp.StatusCode) }
	var data struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil { return nil, err }
	result := make([]map[string]any, len(data.Tags))
	for i, t := range data.Tags {
		result[i] = map[string]any{"name": t}
	}
	return result, nil
}
