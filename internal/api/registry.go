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
	if err != nil { return nil, err }
	// GHCR token-based auth
	if host == "ghcr.io" {
		username, _ := reg["username"].(string)
		password, _ := reg["password"].(string)
		if password == "" { password, _ = reg["token"].(string) }
		if password == "" { return nil, fmt.Errorf("no credentials") }
		// Get token from ghcr.io auth endpoint
		tokenURL := fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull&service=ghcr.io", repo)
		req, _ := http.NewRequest("GET", tokenURL, nil)
		if username == "" { username = "token" }
		req.SetBasicAuth(username, password)
		resp, err := httpClient.Do(req)
		if err != nil { return nil, err }
		defer resp.Body.Close()
		var tokenResp struct{ Token string `json:"token"` }
		json.NewDecoder(resp.Body).Decode(&tokenResp)
		if tokenResp.Token == "" { return nil, fmt.Errorf("auth failed") }
		// Fetch tags with bearer token
		tagsReq, _ := http.NewRequest("GET", fmt.Sprintf("https://ghcr.io/v2/%s/tags/list", repo), nil)
		tagsReq.Header.Set("Authorization", "Bearer "+tokenResp.Token)
		tresp, err := httpClient.Do(tagsReq)
		if err != nil { return nil, err }
		defer tresp.Body.Close()
		var data struct{ Tags []string `json:"tags"` }
		json.NewDecoder(tresp.Body).Decode(&data)
		result := make([]map[string]any, len(data.Tags))
		for i, t := range data.Tags { result[i] = map[string]any{"name": t} }
		return result, nil
	}
	// Generic v2 with basic auth
	username, _ := reg["username"].(string)
	password, _ := reg["password"].(string)
	u := fmt.Sprintf("https://%s/v2/%s/tags/list", host, repo)
	req, _ := http.NewRequest("GET", u, nil)
	if username != "" && password != "" { req.SetBasicAuth(username, password) }
	resp, err := httpClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, fmt.Errorf("registry returned %d", resp.StatusCode) }
	var data struct{ Tags []string `json:"tags"` }
	json.NewDecoder(resp.Body).Decode(&data)
	result := make([]map[string]any, len(data.Tags))
	for i, t := range data.Tags { result[i] = map[string]any{"name": t} }
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
