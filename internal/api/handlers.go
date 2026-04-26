package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/auth"
	"github.com/ccvass/swarmpit-xpx/internal/docker"
	"github.com/ccvass/swarmpit-xpx/internal/store"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp/totp"
	"gopkg.in/yaml.v3"
)

// Version_ is set by main.go from build-time ldflags.
var Version_ = "dev"

func json200(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	slog.Warn("api error", "code", code, "msg", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// #73: helper to check role from request
func reqRole(r *http.Request) string { return r.Header.Get("X-Role") }
func reqUser(r *http.Request) string { return r.Header.Get("X-User") }

func Version(w http.ResponseWriter, r *http.Request) {
	ver, _ := docker.Version()
	ping, _ := docker.Ping()
	apiVer := 0.0
	if v, err := strconv.ParseFloat(ping.APIVersion, 64); err == nil {
		apiVer = v
	}
	instanceName := os.Getenv("SWARMPIT_INSTANCE_NAME")
	var instVal any = nil
	if instanceName != "" {
		instVal = instanceName
	}
	json200(w, map[string]any{
		"name": "swarmpit-xpx", "version": Version_, "revision": nil,
		"initialized": store.AdminExists(), "statistics": true, "instanceName": instVal,
		"docker": map[string]any{"api": apiVer, "engine": ver.Version},
	})
}

func HealthLive(w http.ResponseWriter, r *http.Request)  { json200(w, map[string]string{"status": "UP"}) }
func HealthReady(w http.ResponseWriter, r *http.Request) {
	_, err := docker.Ping()
	s := "UP"
	c := 200
	if err != nil {
		s = "DOWN"
		c = 503
	}
	w.WriteHeader(c)
	json200(w, map[string]any{"status": s, "components": map[string]string{"docker": s, "sqlite": "UP", "stats": "in-memory"}})
}

func Login(w http.ResponseWriter, r *http.Request) {
	u, p, ok := auth.DecodeBasic(r.Header.Get("Authorization"))
	if !ok {
		jsonErr(w, 400, "Missing credentials")
		return
	}
	user := store.AuthenticateUser(u, p)
	if user == nil {
		jsonErr(w, 401, "The username or password you entered is incorrect.")
		return
	}
	// #103: validate TOTP if enabled
	if secret := store.GetTOTPSecret(user.Username); secret != "" {
		otpCode := r.Header.Get("X-TOTP-Code")
		if otpCode == "" {
			jsonErr(w, 403, "TOTP code required")
			return
		}
		if !totp.Validate(otpCode, secret) {
			jsonErr(w, 403, "Invalid TOTP code")
			return
		}
	}
	token, _ := auth.GenerateJWT(user.Username, user.Role)
	json200(w, map[string]string{"token": "Bearer " + token})
}

// #70: OAuth2 login — redirects to provider
func OAuthLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	clientID := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_CLIENT_ID")
	authURL := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_AUTH_URL")
	redirectURI := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_REDIRECT_URI")
	if clientID == "" || authURL == "" {
		jsonErr(w, 400, "OAuth provider not configured: "+provider)
		return
	}
	url := fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&response_type=code&scope=openid+email+profile",
		authURL, clientID, redirectURI)
	http.Redirect(w, r, url, http.StatusFound)
}

// #70: OAuth2 callback — exchanges code for token, creates/finds user
func OAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	code := r.URL.Query().Get("code")
	if code == "" {
		jsonErr(w, 400, "missing code parameter")
		return
	}
	clientID := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_CLIENT_SECRET")
	tokenURL := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_TOKEN_URL")
	userInfoURL := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_USERINFO_URL")
	redirectURI := os.Getenv("OAUTH_" + strings.ToUpper(provider) + "_REDIRECT_URI")
	if clientID == "" || clientSecret == "" || tokenURL == "" {
		jsonErr(w, 500, "OAuth provider not fully configured")
		return
	}

	// Exchange code for access token
	data := fmt.Sprintf("grant_type=authorization_code&code=%s&client_id=%s&client_secret=%s&redirect_uri=%s",
		code, clientID, clientSecret, redirectURI)
	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		jsonErr(w, 500, "token exchange failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)
	if tokenResp.AccessToken == "" {
		jsonErr(w, 401, "OAuth token exchange failed")
		return
	}

	// Fetch user info
	req, _ := http.NewRequest("GET", userInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	uResp, err := http.DefaultClient.Do(req)
	if err != nil {
		jsonErr(w, 500, "userinfo fetch failed")
		return
	}
	defer uResp.Body.Close()
	var userInfo struct {
		Email    string `json:"email"`
		Username string `json:"preferred_username"`
		Name     string `json:"name"`
		Sub      string `json:"sub"`
	}
	json.NewDecoder(uResp.Body).Decode(&userInfo)
	username := userInfo.Username
	if username == "" {
		username = userInfo.Email
	}
	if username == "" {
		username = userInfo.Sub
	}

	// Find or create user
	user := store.GetUserByUsername(username)
	if user == nil {
		defaultRole := os.Getenv("OAUTH_DEFAULT_ROLE")
		if defaultRole == "" {
			defaultRole = "user"
		}
		created, err := store.CreateUser(username, "oauth-"+provider, defaultRole, userInfo.Email)
		if err != nil {
			jsonErr(w, 500, "user creation failed: "+err.Error())
			return
		}
		user = created
	}

	jwtToken, _ := auth.GenerateJWT(user.Username, user.Role)
	json200(w, map[string]string{"token": "Bearer " + jwtToken})
}

func Initialize(w http.ResponseWriter, r *http.Request) {
	if store.AdminExists() {
		jsonErr(w, 400, "Admin already exists")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if _, err := store.CreateUser(body.Username, body.Password, "admin", body.Email); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	json200(w, map[string]string{"status": "ok"})
}

// ── Docker resources with mappers ──

func NodeList(w http.ResponseWriter, r *http.Request) {
	nodes, err := docker.Nodes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapNodes(nodes))
}

func NodeDetail(w http.ResponseWriter, r *http.Request) {
	node, err := docker.Node(resolveNodeID(chi.URLParam(r, "id")))
	if err != nil { jsonErr(w, 404, err.Error()); return }
	cache := getNodeStatsCache()
	json200(w, mapNodeWithStats(node, cache[node.ID]))
}

func ServiceList(w http.ResponseWriter, r *http.Request) {
	svcs, err := docker.Services()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	json200(w, mapServices(svcs, tasks, nets, info))
}

func ServiceInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	svc, err := docker.Service(id)
	if err != nil {
		// Docker SDK accepts name too, but try listing if it fails
		svcs, _ := docker.Services()
		found := false
		for _, s := range svcs {
			if strings.HasPrefix(s.ID, id) || s.Spec.Name == id {
				svc = s
				found = true
				break
			}
		}
		if !found {
			jsonErr(w, 404, "Service not found")
			return
		}
	}
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	json200(w, mapService(svc, tasks, nets, info))
}

func TaskList(w http.ResponseWriter, r *http.Request) {
	tasks, err := docker.Tasks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	nodes, _ := docker.Nodes()
	svcs, _ := docker.Services()
	info, _ := docker.Info()
	json200(w, mapTasks(tasks, nodes, svcs, info))
}

func NetworkList(w http.ResponseWriter, r *http.Request) {
	nets, err := docker.Networks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapNetworks(nets))
}

func SecretList(w http.ResponseWriter, r *http.Request) {
	secrets, err := docker.Secrets()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapSecrets(secrets))
}

func ConfigList(w http.ResponseWriter, r *http.Request) {
	configs, err := docker.Configs()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapConfigs(configs))
}

func VolumeList(w http.ResponseWriter, r *http.Request) {
	vols, err := docker.Volumes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, mapVolumes(vols.Volumes))
}

func StackList(w http.ResponseWriter, r *http.Request) {
	svcs, err := docker.Services()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	stacks := map[string]bool{}
	for _, s := range svcs {
		ns := s.Spec.Labels["com.docker.stack.namespace"]
		if ns != "" {
			stacks[ns] = true
		}
	}
	result := []map[string]any{}
	for name := range stacks {
		result = append(result, mapStack(name, svcs, tasks, nets, info))
	}
	json200(w, result)
}

func StackInfo(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	json200(w, mapStack(name, svcs, tasks, nets, info))
}

func StackServices(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	var result []map[string]any
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			result = append(result, mapService(s, tasks, nets, info))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackTasks(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tasks, _ := docker.Tasks()
	svcs, _ := docker.Services()
	nodes, _ := docker.Nodes()
	info, _ := docker.Info()
	svcIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			svcIDs[s.ID] = true
		}
	}
	var result []map[string]any
	for _, t := range tasks {
		if svcIDs[t.ServiceID] {
			result = append(result, mapTask(t, nodes, svcs, info))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackNetworks(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	nets, _ := docker.Networks()
	// Collect network IDs used by services in this stack
	netIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, n := range s.Spec.TaskTemplate.Networks {
				netIDs[n.Target] = true
			}
		}
	}
	var result []map[string]any
	for _, n := range nets {
		if netIDs[n.ID] {
			result = append(result, mapNetwork(n))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackVolumes(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	// Volumes from stack label + volumes used in mounts by stack services
	svcs, _ := docker.Services()
	vols, _ := docker.Volumes()
	volNames := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, m := range s.Spec.TaskTemplate.ContainerSpec.Mounts {
				if string(m.Type) == "volume" {
					volNames[m.Source] = true
				}
			}
		}
	}
	// Also include volumes with the stack label
	var result []map[string]any
	for _, v := range vols.Volumes {
		if v.Labels["com.docker.stack.namespace"] == name || volNames[v.Name] {
			result = append(result, mapVolume(v))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackConfigs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	configs, _ := docker.Configs()
	// Collect config IDs used by stack services
	cfgIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, c := range s.Spec.TaskTemplate.ContainerSpec.Configs {
				cfgIDs[c.ConfigID] = true
			}
		}
	}
	var result []map[string]any
	for _, c := range configs {
		if c.Spec.Labels["com.docker.stack.namespace"] == name || cfgIDs[c.ID] {
			result = append(result, mapConfig(c))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackSecrets(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	secrets, _ := docker.Secrets()
	// Collect secret IDs used by stack services
	secIDs := map[string]bool{}
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			for _, sec := range s.Spec.TaskTemplate.ContainerSpec.Secrets {
				secIDs[sec.SecretID] = true
			}
		}
	}
	var result []map[string]any
	for _, s := range secrets {
		if s.Spec.Labels["com.docker.stack.namespace"] == name || secIDs[s.ID] {
			result = append(result, mapSecret(s))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func StackFile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	spec := generateStackCompose(name)
	json200(w, map[string]any{"name": name, "spec": spec})
}

func StackCompose(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	json200(w, map[string]string{"compose": generateStackCompose(name)})
}

func generateStackCompose(stackName string) string {
	services, _ := docker.Services()
	networks, _ := docker.Networks()
	netMap := map[string]string{}
	for _, n := range networks {
		netMap[n.ID] = n.Name
	}

	var stackServices []swarm.Service
	for _, s := range services {
		if s.Spec.Labels["com.docker.stack.namespace"] == stackName {
			stackServices = append(stackServices, s)
		}
	}
	if len(stackServices) == 0 {
		return ""
	}

	compose := map[string]any{"version": "3.8"}
	svcMap := map[string]any{}
	allNets := map[string]bool{}

	for _, svc := range stackServices {
		svcName := strings.TrimPrefix(svc.Spec.Name, stackName+"_")
		cs := svc.Spec.TaskTemplate.ContainerSpec
		if cs == nil {
			continue
		}
		tt := svc.Spec.TaskTemplate
		s := map[string]any{"image": cs.Image}

		if len(cs.Command) > 0 {
			s["entrypoint"] = cs.Command
		}
		if len(cs.Args) > 0 {
			s["command"] = cs.Args
		}
		if len(cs.Env) > 0 {
			s["environment"] = cs.Env
		}
		if cs.Hostname != "" {
			s["hostname"] = cs.Hostname
		}
		if cs.User != "" {
			s["user"] = cs.User
		}
		if cs.Dir != "" {
			s["working_dir"] = cs.Dir
		}
		if cs.TTY {
			s["tty"] = true
		}
		if cs.Init != nil && *cs.Init {
			s["init"] = true
		}
		if cs.ReadOnly {
			s["read_only"] = true
		}
		if cs.StopGracePeriod != nil {
			s["stop_grace_period"] = fmt.Sprintf("%ds", int64(*cs.StopGracePeriod)/1e9)
		}
		if len(cs.CapabilityAdd) > 0 {
			s["cap_add"] = cs.CapabilityAdd
		}
		if len(cs.CapabilityDrop) > 0 {
			s["cap_drop"] = cs.CapabilityDrop
		}
		if len(cs.Sysctls) > 0 {
			s["sysctls"] = cs.Sysctls
		}
		if cs.DNSConfig != nil && len(cs.DNSConfig.Nameservers) > 0 {
			s["dns"] = cs.DNSConfig.Nameservers
		}
		if len(cs.Hosts) > 0 {
			s["extra_hosts"] = cs.Hosts
		}
		if cs.Healthcheck != nil && len(cs.Healthcheck.Test) > 0 {
			hc := map[string]any{"test": cs.Healthcheck.Test}
			if cs.Healthcheck.Interval > 0 {
				hc["interval"] = fmt.Sprintf("%ds", int64(cs.Healthcheck.Interval)/1e9)
			}
			if cs.Healthcheck.Timeout > 0 {
				hc["timeout"] = fmt.Sprintf("%ds", int64(cs.Healthcheck.Timeout)/1e9)
			}
			if cs.Healthcheck.Retries > 0 {
				hc["retries"] = cs.Healthcheck.Retries
			}
			if cs.Healthcheck.StartPeriod > 0 {
				hc["start_period"] = fmt.Sprintf("%ds", int64(cs.Healthcheck.StartPeriod)/1e9)
			}
			s["healthcheck"] = hc
		}

		// Ports
		if len(svc.Endpoint.Ports) > 0 {
			var ports []string
			for _, p := range svc.Endpoint.Ports {
				ports = append(ports, fmt.Sprintf("%d:%d/%s", p.PublishedPort, p.TargetPort, p.Protocol))
			}
			s["ports"] = ports
		}

		// Volumes/mounts
		if len(cs.Mounts) > 0 {
			var vols []string
			for _, m := range cs.Mounts {
				entry := fmt.Sprintf("%s:%s", m.Source, m.Target)
				if m.ReadOnly {
					entry += ":ro"
				}
				vols = append(vols, entry)
			}
			s["volumes"] = vols
		}

		// Networks with aliases
		if len(tt.Networks) > 0 {
			netCfg := map[string]any{}
			for _, n := range tt.Networks {
				name := netMap[n.Target]
				if name == "" {
					name = n.Target
				}
				name = strings.TrimPrefix(name, stackName+"_")
				allNets[name] = true
				if len(n.Aliases) > 0 {
					netCfg[name] = map[string]any{"aliases": n.Aliases}
				} else {
					netCfg[name] = nil
				}
			}
			s["networks"] = netCfg
		}

		// Configs
		if len(cs.Configs) > 0 {
			var cfgs []map[string]any
			for _, c := range cs.Configs {
				entry := map[string]any{"source": c.ConfigName}
				if c.File != nil {
					entry["target"] = c.File.Name
				}
				cfgs = append(cfgs, entry)
			}
			s["configs"] = cfgs
		}

		// Secrets
		if len(cs.Secrets) > 0 {
			var secs []map[string]any
			for _, sec := range cs.Secrets {
				entry := map[string]any{"source": sec.SecretName}
				if sec.File != nil {
					entry["target"] = sec.File.Name
				}
				secs = append(secs, entry)
			}
			s["secrets"] = secs
		}

		// Deploy section
		deploy := map[string]any{}
		mode := serviceMode(&svc.Spec)
		if mode == "global" {
			deploy["mode"] = "global"
		} else if mode == "replicated" && svc.Spec.Mode.Replicated != nil && svc.Spec.Mode.Replicated.Replicas != nil {
			deploy["replicas"] = *svc.Spec.Mode.Replicated.Replicas
		}
		if tt.Resources != nil {
			res := map[string]any{}
			if tt.Resources.Limits != nil {
				lim := map[string]any{}
				if tt.Resources.Limits.NanoCPUs > 0 {
					lim["cpus"] = fmt.Sprintf("%.2f", float64(tt.Resources.Limits.NanoCPUs)/1e9)
				}
				if tt.Resources.Limits.MemoryBytes > 0 {
					lim["memory"] = fmt.Sprintf("%dM", tt.Resources.Limits.MemoryBytes/(1024*1024))
				}
				if len(lim) > 0 {
					res["limits"] = lim
				}
			}
			if tt.Resources.Reservations != nil {
				rsv := map[string]any{}
				if tt.Resources.Reservations.NanoCPUs > 0 {
					rsv["cpus"] = fmt.Sprintf("%.2f", float64(tt.Resources.Reservations.NanoCPUs)/1e9)
				}
				if tt.Resources.Reservations.MemoryBytes > 0 {
					rsv["memory"] = fmt.Sprintf("%dM", tt.Resources.Reservations.MemoryBytes/(1024*1024))
				}
				if len(rsv) > 0 {
					res["reservations"] = rsv
				}
			}
			if len(res) > 0 {
				deploy["resources"] = res
			}
		}
		if tt.Placement != nil && len(tt.Placement.Constraints) > 0 {
			deploy["placement"] = map[string]any{"constraints": tt.Placement.Constraints}
		}
		if svc.Spec.UpdateConfig != nil {
			uc := map[string]any{"parallelism": svc.Spec.UpdateConfig.Parallelism}
			if svc.Spec.UpdateConfig.Delay > 0 {
				uc["delay"] = fmt.Sprintf("%ds", int64(svc.Spec.UpdateConfig.Delay)/1e9)
			}
			if svc.Spec.UpdateConfig.Order != "" {
				uc["order"] = svc.Spec.UpdateConfig.Order
			}
			if svc.Spec.UpdateConfig.FailureAction != "" {
				uc["failure_action"] = svc.Spec.UpdateConfig.FailureAction
			}
			deploy["update_config"] = uc
		}
		if svc.Spec.RollbackConfig != nil {
			rc := map[string]any{"parallelism": svc.Spec.RollbackConfig.Parallelism}
			if svc.Spec.RollbackConfig.Order != "" {
				rc["order"] = svc.Spec.RollbackConfig.Order
			}
			deploy["rollback_config"] = rc
		}
		if tt.RestartPolicy != nil {
			rp := map[string]any{}
			if tt.RestartPolicy.Condition != "" {
				rp["condition"] = string(tt.RestartPolicy.Condition)
			}
			if tt.RestartPolicy.MaxAttempts != nil && *tt.RestartPolicy.MaxAttempts > 0 {
				rp["max_attempts"] = *tt.RestartPolicy.MaxAttempts
			}
			if len(rp) > 0 {
				deploy["restart_policy"] = rp
			}
		}
		// Deploy labels (non-stack)
		dLabels := map[string]string{}
		for k, v := range svc.Spec.Labels {
			if !strings.HasPrefix(k, "com.docker.stack") {
				dLabels[k] = v
			}
		}
		if len(dLabels) > 0 {
			deploy["labels"] = dLabels
		}
		if len(deploy) > 0 {
			s["deploy"] = deploy
		}

		// Logging
		if tt.LogDriver != nil && tt.LogDriver.Name != "" {
			lg := map[string]any{"driver": tt.LogDriver.Name}
			if len(tt.LogDriver.Options) > 0 {
				lg["options"] = tt.LogDriver.Options
			}
			s["logging"] = lg
		}

		svcMap[svcName] = s
	}
	compose["services"] = svcMap

	if len(allNets) > 0 {
		netDefs := map[string]any{}
		for n := range allNets {
			// Networks owned by this stack are internal; others are external
			fullName := stackName + "_" + n
			isStackNet := false
			for _, net := range networks {
				if net.Name == n || net.Name == fullName {
					if net.Labels["com.docker.stack.namespace"] == stackName {
						isStackNet = true
					}
					break
				}
			}
			if isStackNet {
				netDefs[n] = nil
			} else {
				netDefs[n] = map[string]any{"external": true}
			}
		}
		compose["networks"] = netDefs
	}

	out, err := yaml.Marshal(compose)
	if err != nil {
		slog.Error("compose marshal failed", "err", err)
		return ""
	}
	return string(out)
}

func ServiceCompose(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	svcID := resolveServiceID(id)
	if svcID == "" {
		svcID = id
	}
	svc, err := docker.Service(svcID)
	if err != nil {
		json200(w, map[string]string{"compose": ""})
		return
	}
	stackName := svc.Spec.Labels["com.docker.stack.namespace"]
	if stackName != "" {
		json200(w, map[string]string{"compose": generateStackCompose(stackName)})
		return
	}
	// Single service — generate minimal compose
	svcName := svc.Spec.Name
	cs := svc.Spec.TaskTemplate.ContainerSpec
	s := map[string]any{"image": cs.Image}
	if len(cs.Env) > 0 {
		s["environment"] = cs.Env
	}
	if len(cs.Command) > 0 {
		s["entrypoint"] = cs.Command
	}
	if len(cs.Args) > 0 {
		s["command"] = cs.Args
	}
	compose := map[string]any{"version": "3.8", "services": map[string]any{svcName: s}}
	out, _ := yaml.Marshal(compose)
	json200(w, map[string]string{"compose": string(out)})
}

// Stats returns cluster resource stats (CPU, memory, disk from node resources)
func Stats(w http.ResponseWriter, r *http.Request) {
	nodes, err := docker.Nodes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	cache := getNodeStatsCache()
	totalCPU := 0.0
	totalMem := int64(0)
	resources := map[string]map[string]any{}
	cpuSum, memSum, diskSum := 0.0, 0.0, 0.0
	memUsed, diskUsed, diskTotal := int64(0), int64(0), int64(0)
	n := 0
	for _, nd := range nodes {
		if nd.Status.State != "ready" { continue }
		cpu := float64(nd.Description.Resources.NanoCPUs) / 1e9
		mem := nd.Description.Resources.MemoryBytes
		totalCPU += cpu
		totalMem += mem
		resources[nd.ID] = map[string]any{"cores": cpu, "memory": mem}
		if s, ok := cache[nd.ID]; ok {
			n++
			if c, ok := s["cpu"].(map[string]any); ok {
				if v, ok := c["usedPercentage"].(float64); ok { cpuSum += v }
			}
			if m, ok := s["memory"].(map[string]any); ok {
				if v, ok := m["usedPercentage"].(float64); ok { memSum += v }
				if v, ok := m["used"].(float64); ok { memUsed += int64(v) }
			}
			if d, ok := s["disk"].(map[string]any); ok {
				if v, ok := d["usedPercentage"].(float64); ok { diskSum += v }
				if v, ok := d["used"].(float64); ok { diskUsed += int64(v) }
				if v, ok := d["total"].(float64); ok { diskTotal += int64(v) }
			}
		}
	}
	cpuAvg, memAvg, diskAvg := 0.0, 0.0, 0.0
	if n > 0 { cpuAvg = cpuSum / float64(n); memAvg = memSum / float64(n); diskAvg = diskSum / float64(n) }
	json200(w, map[string]any{
		"resources": resources,
		"cpu":    map[string]any{"usage": cpuAvg, "cores": totalCPU},
		"memory": map[string]any{"usage": memAvg, "used": memUsed, "total": totalMem},
		"disk":   map[string]any{"usage": diskAvg, "used": diskUsed, "total": diskTotal},
	})
}


// NodeTimeseries returns empty timeseries (stats not implemented in Go backend yet)
func NodeTimeseries(w http.ResponseWriter, r *http.Request) {
	json200(w, getHostTimeseries())
}

// ServiceTaskList returns tasks for a specific service
func ServiceTaskList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Resolve name to ID if needed
	svcs, _ := docker.Services()
	resolvedID := id
	for _, s := range svcs {
		if s.Spec.Name == id || strings.HasPrefix(s.ID, id) {
			resolvedID = s.ID
			break
		}
	}
	tasks, err := docker.Tasks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	nodes, _ := docker.Nodes()
	info, _ := docker.Info()
	var result []map[string]any
	for _, t := range tasks {
		if t.ServiceID == resolvedID {
			result = append(result, mapTask(t, nodes, svcs, info))
		}
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

func TaskInfo(w http.ResponseWriter, r *http.Request) {
	tasks, err := docker.Tasks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	nodes, _ := docker.Nodes()
	svcs, _ := docker.Services()
	info, _ := docker.Info()
	id := chi.URLParam(r, "id")
	for _, t := range tasks {
		if t.ID == id {
			json200(w, mapTask(t, nodes, svcs, info))
			return
		}
	}
	jsonErr(w, 404, "Task not found")
}

func NetworkInfo(w http.ResponseWriter, r *http.Request) {
	nets, err := docker.Networks()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, n := range nets {
		if n.ID == id || n.Name == id {
			json200(w, mapNetwork(n))
			return
		}
	}
	jsonErr(w, 404, "Network not found")
}

func VolumeInfo(w http.ResponseWriter, r *http.Request) {
	vols, err := docker.Volumes()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, v := range vols.Volumes {
		if v.Name == id {
			json200(w, mapVolume(v))
			return
		}
	}
	jsonErr(w, 404, "Volume not found")
}

func SecretInfo(w http.ResponseWriter, r *http.Request) {
	secrets, err := docker.Secrets()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, s := range secrets {
		if s.ID == id || s.Spec.Name == id {
			json200(w, mapSecret(s))
			return
		}
	}
	jsonErr(w, 404, "Secret not found")
}

func ConfigInfo(w http.ResponseWriter, r *http.Request) {
	configs, err := docker.Configs()
	if err != nil { jsonErr(w, 500, err.Error()); return }
	id := chi.URLParam(r, "id")
	for _, c := range configs {
		if c.ID == id || c.Spec.Name == id {
			json200(w, mapConfig(c))
			return
		}
	}
	jsonErr(w, 404, "Config not found")
}

func resolveServiceID(id string) string {
	svcs, _ := docker.Services()
	for _, s := range svcs {
		if s.Spec.Name == id || strings.HasPrefix(s.ID, id) { return s.ID }
	}
	return id
}


func resolveNodeID(id string) string {
	nodes, _ := docker.Nodes()
	for _, n := range nodes {
		if n.ID == id || n.Description.Hostname == id {
			return n.ID
		}
	}
	return id
}

func ServiceLogs(w http.ResponseWriter, r *http.Request) {
	svcID := resolveServiceID(chi.URLParam(r, "id"))
	if r.URL.Query().Get("follow") == "true" {
		flusher, ok := w.(http.Flusher)
		if !ok { jsonErr(w, 500, "streaming not supported"); return }
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil { jsonErr(w, 500, err.Error()); return }
		reader, err := cli.ServiceLogs(r.Context(), svcID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Tail: "50", Timestamps: true})
		if err != nil { jsonErr(w, 500, err.Error()); return }
		defer reader.Close()
		// Demultiplex: read 8-byte header + payload per frame
		hdr := make([]byte, 8)
		for {
			if _, err := io.ReadFull(reader, hdr); err != nil {
				return
			}
			size := int(hdr[4])<<24 | int(hdr[5])<<16 | int(hdr[6])<<8 | int(hdr[7])
			if size <= 0 || size > 1<<20 {
				continue
			}
			payload := make([]byte, size)
			if _, err := io.ReadFull(reader, payload); err != nil {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(string(payload)))
			flusher.Flush()
		}
	}
	logs, err := docker.ServiceLogs(svcID, r.URL.Query().Get("tail"))
	if err != nil { jsonErr(w, 500, err.Error()); return }
	// Parse raw logs into array of {line, task, timestamp} objects
	// Docker log format: "TIMESTAMP TASKID MESSAGE" per line
	entries := []map[string]any{}
	for _, raw := range strings.Split(logs, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" { continue }
		entry := map[string]any{"line": raw}
		// Try to parse "TIMESTAMP TASKNAME.SLOT.TASKID@NODE MESSAGE"
		parts := strings.SplitN(raw, " ", 3)
		if len(parts) >= 2 {
			entry["timestamp"] = parts[0]
			// Second part may contain task info like "serviceName.1.taskID@node"
			taskParts := strings.SplitN(parts[1], ".", 3)
			if len(taskParts) >= 3 {
				atIdx := strings.Index(taskParts[2], "@")
				if atIdx > 0 {
					entry["task"] = taskParts[2][:atIdx]
				} else {
					entry["task"] = taskParts[2]
				}
			}
			if len(parts) >= 3 {
				entry["line"] = parts[2]
			}
		}
		entries = append(entries, entry)
	}
	json200(w, entries)
}

func ServiceDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteService(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func SecretDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteSecret(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func ConfigDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteConfig(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func NetworkDelete(w http.ResponseWriter, r *http.Request) {
	if err := docker.DeleteNetwork(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	w.WriteHeader(200)
}

func WebhookTrigger(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	svcID, ok := store.FindWebhook(token)
	if !ok { jsonErr(w, 404, "Webhook not found"); return }

	// Parse body to detect image info for auto-deploy (#61)
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	webhookImage := ""
	if pd, ok := body["push_data"].(map[string]any); ok {
		if repo, ok := body["repository"].(map[string]any); ok {
			tag, _ := pd["tag"].(string)
			repoName, _ := repo["repo_name"].(string)
			if repoName != "" && tag != "" { webhookImage = repoName + ":" + tag }
		}
	} else if img, ok := body["image"].(string); ok && img != "" {
		webhookImage = img
	}

	// If image detected, find and update all services using it
	if webhookImage != "" {
		svcs, _ := docker.Services()
		for _, s := range svcs {
			svcImg := s.Spec.TaskTemplate.ContainerSpec.Image
			imgBase := strings.SplitN(svcImg, "@", 2)[0]
			if imgBase == webhookImage || strings.HasPrefix(imgBase, webhookImage) {
				s.Spec.TaskTemplate.ForceUpdate++
				docker.UpdateService(s.ID, s.Version, s.Spec)
				store.RecordAudit("webhook", "auto-deploy", "service", s.Spec.Name)
			}
		}
	}

	// Always force-update the webhook's own service
	svc, err := docker.Service(svcID)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	svc.Spec.TaskTemplate.ForceUpdate++
	if err := docker.UpdateService(svcID, svc.Version, svc.Spec); err != nil { jsonErr(w, 500, err.Error()); return }
	store.RecordAudit("webhook", "trigger", "service", svc.Spec.Name)
	json200(w, map[string]string{"status": "triggered"})
}

func AuditList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit == 0 { limit = 50 }
	json200(w, store.AuditEntries(limit, offset))
}

func GitDeploy(w http.ResponseWriter, r *http.Request) {
	GitStackCreate(w, r)
}

// Node tasks
func NodeTasks(w http.ResponseWriter, r *http.Request) {
	id := resolveNodeID(chi.URLParam(r, "id"))
	tasks, _ := docker.Tasks()
	nodes, _ := docker.Nodes()
	svcs, _ := docker.Services()
	info, _ := docker.Info()
	var result []map[string]any
	for _, t := range tasks {
		if t.NodeID == id { result = append(result, mapTask(t, nodes, svcs, info)) }
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

// Resource-linked services
func NetworkServices(w http.ResponseWriter, r *http.Request) { linkedServices(w, r, "network") }
func VolumeServices(w http.ResponseWriter, r *http.Request)  { linkedServices(w, r, "volume") }
func SecretServices(w http.ResponseWriter, r *http.Request)  { linkedServices(w, r, "secret") }
func ConfigServices(w http.ResponseWriter, r *http.Request)  { linkedServices(w, r, "config") }

func linkedServices(w http.ResponseWriter, r *http.Request, resType string) {
	id := chi.URLParam(r, "id")
	svcs, _ := docker.Services()
	tasks, _ := docker.Tasks()
	nets, _ := docker.Networks()
	info, _ := docker.Info()
	var result []map[string]any
	for _, s := range svcs {
		spec := s.Spec
		match := false
		switch resType {
		case "network":
			for _, n := range spec.TaskTemplate.Networks {
				if n.Target == id { match = true; break }
			}
			// Also check by name
			if !match {
				for _, n := range nets {
					if n.Name == id {
						for _, sn := range spec.TaskTemplate.Networks {
							if sn.Target == n.ID { match = true; break }
						}
					}
				}
			}
		case "volume":
			for _, m := range spec.TaskTemplate.ContainerSpec.Mounts {
				if string(m.Type) == "volume" && m.Source == id { match = true; break }
			}
		case "secret":
			for _, sec := range spec.TaskTemplate.ContainerSpec.Secrets {
				if sec.SecretID == id || sec.SecretName == id { match = true; break }
			}
		case "config":
			for _, cfg := range spec.TaskTemplate.ContainerSpec.Configs {
				if cfg.ConfigID == id || cfg.ConfigName == id { match = true; break }
			}
		}
		if match { result = append(result, mapService(s, tasks, nets, info)) }
	}
	if result == nil { result = []map[string]any{} }
	json200(w, result)
}

// Service sub-resources
func ServiceNetworks(w http.ResponseWriter, r *http.Request) {
	svc, err := docker.Service(chi.URLParam(r, "id"))
	if err != nil { json200(w, []any{}); return }
	nets, _ := docker.Networks()
	netMap := map[string]any{}
	for _, n := range nets { netMap[n.ID] = mapNetwork(n) }
	var result []any
	for _, n := range svc.Spec.TaskTemplate.Networks {
		if v, ok := netMap[n.Target]; ok { result = append(result, v) }
	}
	if result == nil { result = []any{} }
	json200(w, result)
}

// Timeseries (empty — no historical data in Go backend yet)
func ServicesTsCPU(w http.ResponseWriter, r *http.Request) { json200(w, getServiceTimeseries("cpu")) }
func ServicesTsMemory(w http.ResponseWriter, r *http.Request) { json200(w, getServiceTimeseries("memory")) }
func TaskTimeseries(w http.ResponseWriter, r *http.Request) { json200(w, getTaskTimeseries(chi.URLParam(r, "id"))) }

// Plugins and placement
func LabelsService(w http.ResponseWriter, r *http.Request) {
	svcs, _ := docker.Services()
	labels := map[string]bool{}
	for _, s := range svcs {
		for k := range s.Spec.Labels { labels[k] = true }
	}
	result := []string{}
	for k := range labels { result = append(result, k) }
	json200(w, result)
}

func PluginNetwork(w http.ResponseWriter, r *http.Request) {
	json200(w, []string{"bridge", "host", "overlay", "macvlan"})
}

func PluginVolume(w http.ResponseWriter, r *http.Request) {
	json200(w, []string{"local"})
}

func PluginLog(w http.ResponseWriter, r *http.Request) {
	json200(w, []string{"json-file", "syslog", "journald", "gelf", "fluentd", "awslogs", "splunk", "none"})
}

func Placement(w http.ResponseWriter, r *http.Request) {
	nodes, _ := docker.Nodes()
	var result []string
	for _, n := range nodes {
		result = append(result, "node.hostname == "+n.Description.Hostname)
		for k, v := range n.Spec.Labels {
			result = append(result, "node.labels."+k+" == "+v)
		}
	}
	json200(w, result)
}

// ── CRUD handlers (#36) ──

// sanitizeImageRef strips empty or malformed digests from image references (#86)
func sanitizeImageRef(img string) string {
	if strings.HasSuffix(img, "@") {
		return strings.TrimSuffix(img, "@")
	}
	if idx := strings.Index(img, "@"); idx >= 0 {
		digest := img[idx+1:]
		if digest == "" || !strings.Contains(digest, ":") {
			return img[:idx]
		}
	}
	return img
}

func ServiceCreate(w http.ResponseWriter, r *http.Request) {
	var spec swarm.ServiceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		jsonErr(w, 400, "invalid service spec: "+err.Error())
		return
	}
	if spec.TaskTemplate.ContainerSpec != nil {
		spec.TaskTemplate.ContainerSpec.Image = sanitizeImageRef(spec.TaskTemplate.ContainerSpec.Image)
	}
	resp, err := docker.CreateService(spec)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	store.RecordAudit(reqUser(r), "create", "service", spec.Name)
	json200(w, map[string]string{"id": resp.ID})
}

func ServiceUpdate(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil {
		jsonErr(w, 404, "service not found")
		return
	}
	var spec swarm.ServiceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		jsonErr(w, 400, "invalid service spec: "+err.Error())
		return
	}
	if spec.TaskTemplate.ContainerSpec != nil {
		spec.TaskTemplate.ContainerSpec.Image = sanitizeImageRef(spec.TaskTemplate.ContainerSpec.Image)
	}
	if err := docker.UpdateService(id, svc.Version, spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	store.RecordAudit(reqUser(r), "update", "service", spec.Name)
	json200(w, map[string]string{"status": "updated"})
}

func StackCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Spec string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Spec == "" {
		jsonErr(w, 400, "name and spec (YAML) required")
		return
	}
	out, err := deployStack(body.Name, body.Spec)
	if err != nil {
		jsonErr(w, 500, out)
		return
	}
	json200(w, map[string]string{"status": "deployed", "output": out})
}

// #89: Bulk stack import
func StackImport(w http.ResponseWriter, r *http.Request) {
	var stacks []struct {
		Name string `json:"name"`
		Spec string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&stacks); err != nil || len(stacks) == 0 {
		jsonErr(w, 400, "array of {name, spec} required")
		return
	}
	results := make([]map[string]string, 0, len(stacks))
	for _, s := range stacks {
		if s.Name == "" || s.Spec == "" {
			results = append(results, map[string]string{"name": s.Name, "status": "error", "error": "name and spec required"})
			continue
		}
		out, err := deployStack(s.Name, s.Spec)
		if err != nil {
			results = append(results, map[string]string{"name": s.Name, "status": "error", "error": out})
		} else {
			results = append(results, map[string]string{"name": s.Name, "status": "deployed"})
		}
	}
	json200(w, results)
}

func StackUpdate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		Spec string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Spec == "" {
		jsonErr(w, 400, "spec (YAML) required")
		return
	}
	out, err := deployStack(name, body.Spec)
	if err != nil {
		jsonErr(w, 500, out)
		return
	}
	json200(w, map[string]string{"status": "updated", "output": out})
}

func deployStack(name, spec string) (string, error) {
	cmd := exec.Command("docker", "stack", "deploy", "--with-registry-auth", "-c", "-", name)
	cmd.Stdin = strings.NewReader(spec)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func StackDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	out, err := exec.Command("docker", "stack", "rm", name).CombinedOutput()
	if err != nil {
		jsonErr(w, 500, string(out))
		return
	}
	json200(w, map[string]string{"status": "removed", "output": string(out)})
}

func NetworkCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NetworkName string `json:"networkName"`
		Driver      string `json:"driver"`
		Internal    bool   `json:"internal"`
		Attachable  bool   `json:"attachable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.NetworkName == "" {
		jsonErr(w, 400, "networkName required")
		return
	}
	if body.Driver == "" {
		body.Driver = "overlay"
	}
	resp, err := docker.CreateNetwork(body.NetworkName, body.Driver, body.Internal, body.Attachable)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func VolumeCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VolumeName string `json:"volumeName"`
		Driver     string `json:"driver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VolumeName == "" {
		jsonErr(w, 400, "volumeName required")
		return
	}
	if body.Driver == "" {
		body.Driver = "local"
	}
	v, err := docker.CreateVolume(body.VolumeName, body.Driver)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"name": v.Name})
}

func SecretCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SecretName string `json:"secretName"`
		Data       string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SecretName == "" {
		jsonErr(w, 400, "secretName required")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		jsonErr(w, 400, "data must be base64 encoded")
		return
	}
	resp, err := docker.CreateSecret(swarm.SecretSpec{
		Annotations: swarm.Annotations{Name: body.SecretName},
		Data:        data,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func ConfigCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConfigName string `json:"configName"`
		Data       string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ConfigName == "" {
		jsonErr(w, 400, "configName required")
		return
	}
	resp, err := docker.CreateConfig(swarm.ConfigSpec{
		Annotations: swarm.Annotations{Name: body.ConfigName},
		Data:        []byte(body.Data),
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": resp.ID})
}

func SecretUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sec, err := docker.SecretInspect(id)
	if err != nil {
		jsonErr(w, 404, "secret not found")
		return
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	data, err := base64.StdEncoding.DecodeString(body.Data)
	if err != nil {
		jsonErr(w, 400, "data must be base64 encoded")
		return
	}
	sec.Spec.Data = data
	if err := docker.UpdateSecret(id, sec.Version, sec.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "updated"})
}

func ConfigUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, err := docker.ConfigInspect(id)
	if err != nil {
		jsonErr(w, 404, "config not found")
		return
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	cfg.Spec.Data = []byte(body.Data)
	if err := docker.UpdateConfig(id, cfg.Version, cfg.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "updated"})
}

// ── Redeploy & Rollback (#37) ──

func ServiceRedeploy(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil {
		jsonErr(w, 404, "service not found")
		return
	}
	svc.Spec.TaskTemplate.ForceUpdate++
	if err := docker.UpdateService(id, svc.Version, svc.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "redeployed"})
}

func ServiceRollback(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil {
		jsonErr(w, 404, "service not found")
		return
	}
	if svc.PreviousSpec == nil {
		jsonErr(w, 400, "no previous spec available for rollback")
		return
	}
	if err := docker.UpdateService(id, svc.Version, *svc.PreviousSpec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "rolled back"})
}

// ── #43 Service stop ──

func ServiceStop(w http.ResponseWriter, r *http.Request) {
	id := resolveServiceID(chi.URLParam(r, "id"))
	svc, err := docker.Service(id)
	if err != nil { jsonErr(w, 404, "service not found"); return }
	zero := uint64(0)
	svc.Spec.Mode.Replicated.Replicas = &zero
	if err := docker.UpdateService(id, svc.Version, svc.Spec); err != nil {
		jsonErr(w, 500, err.Error()); return
	}
	json200(w, map[string]string{"status": "stopped"})
}

// ── #44 Stack redeploy/rollback ──

func StackRedeploy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	count := 0
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name {
			s.Spec.TaskTemplate.ForceUpdate++
			if err := docker.UpdateService(s.ID, s.Version, s.Spec); err == nil { count++ }
		}
	}
	json200(w, map[string]any{"status": "redeployed", "services": count})
}

func StackRollback(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	count := 0
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name && s.PreviousSpec != nil {
			if err := docker.UpdateService(s.ID, s.Version, *s.PreviousSpec); err == nil { count++ }
		}
	}
	json200(w, map[string]any{"status": "rolled back", "services": count})
}

// ── #45 Registry management ──

// redactRegistryCreds removes sensitive fields from registry data for API responses (#77)
func redactRegistryCreds(reg map[string]any) map[string]any {
	out := make(map[string]any, len(reg))
	for k, v := range reg {
		out[k] = v
	}
	if _, ok := out["password"]; ok {
		out["password"] = "••••••"
	}
	if _, ok := out["token"]; ok {
		out["token"] = "••••••"
	}
	if _, ok := out["secret"]; ok {
		out["secret"] = "••••••"
	}
	return out
}

func RegistryList(w http.ResponseWriter, r *http.Request) {
	regType := chi.URLParam(r, "type")
	regs, _ := store.ListRegistries(regType)
	for i := range regs {
		regs[i] = redactRegistryCreds(regs[i])
	}
	json200(w, regs)
}

func RegistryCreate(w http.ResponseWriter, r *http.Request) {
	regType := chi.URLParam(r, "type")
	var reg map[string]any
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		jsonErr(w, 400, "invalid body"); return
	}
	reg["type"] = regType
	id, err := store.CreateRegistry(reg)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	reg["id"] = id
	json200(w, redactRegistryCreds(reg))
}

func RegistryInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reg, err := store.GetRegistry(id)
	if err != nil { jsonErr(w, 404, "registry not found"); return }
	json200(w, redactRegistryCreds(reg))
}

func RegistryUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var reg map[string]any
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		jsonErr(w, 400, "invalid body"); return
	}
	if err := store.UpdateRegistry(id, reg); err != nil {
		jsonErr(w, 500, err.Error()); return
	}
	json200(w, map[string]string{"status": "updated"})
}

func RegistryDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := store.DeleteRegistry(id); err != nil {
		jsonErr(w, 500, err.Error()); return
	}
	json200(w, map[string]string{"status": "deleted"})
}

func RegistryRepositories(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	reg, err := store.GetRegistry(id)
	if err != nil { jsonErr(w, 404, "registry not found"); return }
	repos, err := listRegistryRepos(reg)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, repos)
}

// ── #46 Image tags + DockerHub search ──

func PublicRepositories(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" { json200(w, []any{}); return }
	repos, err := searchDockerHub(query)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, repos)
}

func RepositoryTags(w http.ResponseWriter, r *http.Request) {
	repo := r.URL.Query().Get("repository")
	if repo == "" {
		json200(w, []any{})
		return
	}
	fetchAndReturnTags(w, repo)
}

func ImageTags(w http.ResponseWriter, r *http.Request) {
	repo := chi.URLParam(r, "*")
	if repo == "" {
		json200(w, []any{})
		return
	}
	fetchAndReturnTags(w, repo)
}

func fetchAndReturnTags(w http.ResponseWriter, repo string) {
	// Check if it's a private registry image (contains a dot in the first segment)
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) >= 2 && strings.Contains(parts[0], ".") {
		host := parts[0]
		tags, err := fetchAuthenticatedTags(host, parts[1])
		if err == nil { json200(w, tagsToStrings(tags)); return }
	}
	// Try DockerHub, then generic v2
	tags, err := fetchDockerHubTags(repo)
	if err != nil {
		tags, err = fetchRegistryV2Tags(repo)
		if err != nil { json200(w, []string{}); return }
	}
	json200(w, tagsToStrings(tags))
}

func tagsToStrings(tags []map[string]any) []string {
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		if name, ok := t["name"].(string); ok {
			result = append(result, name)
		}
	}
	return result
}

// ── #47 Node edit — labels and availability ──

func NodeEdit(w http.ResponseWriter, r *http.Request) {
	id := resolveNodeID(chi.URLParam(r, "id"))
	node, err := docker.Node(id)
	if err != nil {
		jsonErr(w, 404, "node not found")
		return
	}
	var body struct {
		Role         string            `json:"role"`
		Availability string            `json:"availability"`
		Labels       []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid body")
		return
	}
	if body.Role != "" {
		node.Spec.Role = swarm.NodeRole(body.Role)
	}
	if body.Availability != "" {
		node.Spec.Availability = swarm.NodeAvailability(body.Availability)
	}
	if body.Labels != nil {
		labels := map[string]string{}
		for _, l := range body.Labels {
			labels[l.Name] = l.Value
		}
		node.Spec.Labels = labels
	}
	if err := docker.NodeUpdate(id, node.Version, node.Spec); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "updated"})
}

// ── #48 Stack activate/deactivate ──

func StackActivate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	saved := store.GetDeactivatedReplicas(name)
	count := 0
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name && s.Spec.Mode.Replicated != nil {
			replicas := uint64(1)
			if v, ok := saved[s.ID]; ok && v > 0 {
				replicas = v
			}
			s.Spec.Mode.Replicated.Replicas = &replicas
			if err := docker.UpdateService(s.ID, s.Version, s.Spec); err == nil {
				count++
			}
		}
	}
	store.ClearDeactivatedReplicas(name)
	json200(w, map[string]any{"status": "activated", "services": count})
}

func StackDeactivate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	svcs, _ := docker.Services()
	count := 0
	zero := uint64(0)
	for _, s := range svcs {
		if s.Spec.Labels["com.docker.stack.namespace"] == name && s.Spec.Mode.Replicated != nil {
			// Save current replica count before deactivation (#106)
			if s.Spec.Mode.Replicated.Replicas != nil {
				store.SaveDeactivatedReplicas(name, s.ID, *s.Spec.Mode.Replicated.Replicas)
			}
			s.Spec.Mode.Replicated.Replicas = &zero
			if err := docker.UpdateService(s.ID, s.Version, s.Spec); err == nil {
				count++
			}
		}
	}
	json200(w, map[string]any{"status": "deactivated", "services": count})
}

// ── #49 Password change + user profile ──

func Me(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-User")
	user := store.GetUserByUsername(username)
	if user == nil {
		jsonErr(w, 404, "user not found")
		return
	}
	resp := map[string]any{
		"_id": user.ID, "username": user.Username, "role": user.Role, "email": user.Email,
		"serviceDashboard": store.GetPins(username, "service"),
		"nodeDashboard":    store.GetPins(username, "node"),
	}
	json200(w, resp)
}

func PasswordChange(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-User")
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		jsonErr(w, 400, "password required")
		return
	}
	if err := store.UpdatePassword(username, body.Password); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "ok"})
}

// ── #50 Webhooks CRUD ──

func WebhookList(w http.ResponseWriter, r *http.Request) {
	json200(w, store.ListWebhooks())
}

func WebhookCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ServiceName string `json:"serviceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ServiceName == "" {
		jsonErr(w, 400, "serviceName required")
		return
	}
	json200(w, store.CreateWebhook(body.ServiceName))
}

func WebhookDelete(w http.ResponseWriter, r *http.Request) {
	if err := store.DeleteWebhook(chi.URLParam(r, "id")); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "deleted"})
}

// ── #39 User management (admin-only) ──

func UserList(w http.ResponseWriter, r *http.Request) {
	json200(w, store.ListUsers())
}

func UserCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		jsonErr(w, 400, "username and password required"); return
	}
	if body.Role == "" { body.Role = "user" }
	user, err := store.CreateUser(body.Username, body.Password, body.Role, body.Email)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, user)
}

func UserUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, 400, "invalid body"); return
	}
	if err := store.UpdateUser(id, body.Username, body.Email, body.Role); err != nil {
		jsonErr(w, 500, err.Error()); return
	}
	json200(w, map[string]string{"status": "updated"})
}

func UserDelete(w http.ResponseWriter, r *http.Request) {
	if err := store.DeleteUser(chi.URLParam(r, "id")); err != nil {
		jsonErr(w, 500, err.Error()); return
	}
	json200(w, map[string]string{"status": "deleted"})
}

// ── #51 Dashboard pinning ──

func DashboardPinService(w http.ResponseWriter, r *http.Request) {
	pinned := store.TogglePin(r.Header.Get("X-User"), "service", chi.URLParam(r, "id"))
	json200(w, map[string]bool{"pinned": pinned})
}

func DashboardPinNode(w http.ResponseWriter, r *http.Request) {
	pinned := store.TogglePin(r.Header.Get("X-User"), "node", chi.URLParam(r, "id"))
	json200(w, map[string]bool{"pinned": pinned})
}

// ── #52 Swagger/OpenAPI docs ──

// #103: TOTP setup with real TOTP secret generation
func TOTPSetup(w http.ResponseWriter, r *http.Request) {
	username := reqUser(r)
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "swarmpit-xpx",
		AccountName: username,
	})
	if err != nil {
		jsonErr(w, 500, "totp generation failed: "+err.Error())
		return
	}
	if err := store.SetTOTPSecret(username, key.Secret()); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"secret": key.Secret(), "url": key.URL(), "issuer": "swarmpit-xpx", "account": username})
}

func TOTPDisable(w http.ResponseWriter, r *http.Request) {
	username := reqUser(r)
	// Require valid OTP code to disable
	var body struct {
		Code string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	secret := store.GetTOTPSecret(username)
	if secret != "" && body.Code != "" {
		if !totp.Validate(body.Code, secret) {
			jsonErr(w, 403, "invalid TOTP code")
			return
		}
	}
	if err := store.ClearTOTPSecret(username); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "disabled"})
}

// #99: Team permissions CRUD
func TeamPermissionList(w http.ResponseWriter, r *http.Request) {
	json200(w, store.ListTeamPermissions())
}

func TeamPermissionCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TeamName     string `json:"teamName"`
		Username     string `json:"username"`
		StackPattern string `json:"stackPattern"`
		Role         string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TeamName == "" || body.Username == "" {
		jsonErr(w, 400, "teamName and username required")
		return
	}
	if body.StackPattern == "" {
		body.StackPattern = "*"
	}
	if body.Role == "" {
		body.Role = "user"
	}
	id, err := store.CreateTeamPermission(body.TeamName, body.Username, body.StackPattern, body.Role)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": id, "status": "created"})
}

func TeamPermissionDelete(w http.ResponseWriter, r *http.Request) {
	if err := store.DeleteTeamPermission(chi.URLParam(r, "id")); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "deleted"})
}

// #100: Multi-cluster CRUD
func ClusterList(w http.ResponseWriter, r *http.Request) {
	json200(w, store.ListClusters())
}

func ClusterCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		DockerHost string `json:"dockerHost"`
		TLSCa      string `json:"tlsCa"`
		TLSCert    string `json:"tlsCert"`
		TLSKey     string `json:"tlsKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.DockerHost == "" {
		jsonErr(w, 400, "name and dockerHost required")
		return
	}
	id, err := store.CreateCluster(body.Name, body.DockerHost, body.TLSCa, body.TLSCert, body.TLSKey)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"id": id, "status": "created"})
}

func ClusterDelete(w http.ResponseWriter, r *http.Request) {
	if err := store.DeleteCluster(chi.URLParam(r, "id")); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "deleted"})
}

func ClusterActivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := store.SetActiveCluster(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	// Reinitialize Docker client to point at the activated cluster (#104)
	_, host := store.GetActiveCluster()
	if err := docker.InitWithHost(host); err != nil {
		jsonErr(w, 500, "cluster activation failed: "+err.Error())
		return
	}
	json200(w, map[string]string{"status": "activated"})
}

func SwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html><html><head><title>Swarmpit XPX API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head><body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url:"/api-docs/swagger.json",dom_id:"#swagger-ui"})</script>
</body></html>`))
}

func SwaggerJSON(w http.ResponseWriter, r *http.Request) {
	paths := map[string]any{}
	chi.Walk(r.Context().Value(chiRouterKey).(chi.Router), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, ok := paths[route]; !ok { paths[route] = map[string]any{} }
		paths[route].(map[string]any)[strings.ToLower(method)] = map[string]any{
			"summary": method + " " + route, "responses": map[string]any{"200": map[string]string{"description": "OK"}},
		}
		return nil
	})
	json200(w, map[string]any{
		"openapi": "3.0.0",
		"info":    map[string]string{"title": "Swarmpit XPX API", "version": "2.5.0"},
		"paths":   paths,
	})
}

type ctxKey string

const chiRouterKey ctxKey = "chiRouter"

// ── #54 Alerting ──

func AlertRuleList(w http.ResponseWriter, r *http.Request) { json200(w, store.ListAlertRules()) }

func AlertRuleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type      string  `json:"type"`
		Condition string  `json:"condition"`
		Threshold float64 `json:"threshold"`
		Channel   string  `json:"channel"`
		Target    string  `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Type == "" {
		jsonErr(w, 400, "type required"); return
	}
	json200(w, store.CreateAlertRule(body.Type, body.Condition, body.Threshold, body.Channel, body.Target))
}

func AlertRuleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Type      string  `json:"type"`
		Condition string  `json:"condition"`
		Threshold float64 `json:"threshold"`
		Channel   string  `json:"channel"`
		Target    string  `json:"target"`
		Enabled   bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { jsonErr(w, 400, "invalid body"); return }
	if err := store.UpdateAlertRule(id, body.Type, body.Condition, body.Threshold, body.Channel, body.Target, body.Enabled); err != nil {
		jsonErr(w, 500, err.Error()); return
	}
	json200(w, map[string]string{"status": "updated"})
}

func AlertRuleDelete(w http.ResponseWriter, r *http.Request) {
	if err := store.DeleteAlertRule(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, map[string]string{"status": "deleted"})
}

func AlertHistoryList(w http.ResponseWriter, r *http.Request) { json200(w, store.ListAlertHistory()) }

// StartAlertChecker runs background alert evaluation every 30s
func StartAlertChecker() {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			checkAlerts()
		}
	}()
}

func checkAlerts() {
	rules := store.ListAlertRules()
	cache := getNodeStatsCache()
	for _, rule := range rules {
		enabled, _ := rule["enabled"].(bool)
		if !enabled { continue }
		typ, _ := rule["type"].(string)
		threshold, _ := rule["threshold"].(float64)
		channel, _ := rule["channel"].(string)
		target, _ := rule["target"].(string)
		ruleID, _ := rule["id"].(string)

		for nodeID, stats := range cache {
			var val float64
			switch typ {
			case "cpu_high":
				if c, ok := stats["cpu"].(map[string]any); ok { val, _ = c["usedPercentage"].(float64) }
			case "memory_high":
				if m, ok := stats["memory"].(map[string]any); ok { val, _ = m["usedPercentage"].(float64) }
			case "disk_high":
				if d, ok := stats["disk"].(map[string]any); ok { val, _ = d["usedPercentage"].(float64) }
			}
			if val > threshold {
				msg := fmt.Sprintf("%s: node %s at %.1f%% (threshold %.1f%%)", typ, nodeID, val, threshold)
				store.RecordAlertHistory(ruleID, msg)
				if channel == "webhook" && target != "" {
					go sendWebhookAlert(target, msg)
				}
			}
		}
	}
}

func sendWebhookAlert(url, msg string) {
	payload, _ := json.Marshal(map[string]string{"alert": msg, "timestamp": time.Now().UTC().Format(time.RFC3339)})
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil { slog.Warn("alert webhook failed", "url", url, "err", err); return }
	resp.Body.Close()
}

// ── #60 Per-service timeseries ──

func ServiceTimeseries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Resolve service ID/prefix to name
	svcs, _ := docker.Services()
	svcName := id
	for _, s := range svcs {
		if s.ID == id || strings.HasPrefix(s.ID, id) || s.Spec.Name == id {
			svcName = s.Spec.Name
			break
		}
	}
	json200(w, getServiceTimeseriesByName(svcName))
}

// ── #55 Backup/restore ──

func BackupHandler(w http.ResponseWriter, r *http.Request) {
	data, err := store.ExportAll()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=swarmpit-backup.json")
	json200(w, data)
}

func RestoreHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		jsonErr(w, 400, "invalid JSON: "+err.Error())
		return
	}
	if err := store.ImportAll(data); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	json200(w, map[string]string{"status": "restored"})
}

// ── #56 Service templates ──

func TemplateList(w http.ResponseWriter, r *http.Request) { json200(w, store.ListTemplates()) }

func TemplateCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Spec        string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		jsonErr(w, 400, "name required"); return
	}
	json200(w, store.CreateTemplate(body.Name, body.Description, body.Spec))
}

func TemplateDeploy(w http.ResponseWriter, r *http.Request) {
	tpl, err := store.GetTemplate(chi.URLParam(r, "id"))
	if err != nil { jsonErr(w, 404, "template not found"); return }
	spec, _ := tpl["spec"].(string)
	var svcSpec swarm.ServiceSpec
	if err := json.Unmarshal([]byte(spec), &svcSpec); err != nil {
		jsonErr(w, 400, "invalid template spec: "+err.Error()); return
	}
	resp, err := docker.CreateService(svcSpec)
	if err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, map[string]string{"id": resp.ID, "status": "deployed"})
}

func TemplateDelete(w http.ResponseWriter, r *http.Request) {
	if err := store.DeleteTemplate(chi.URLParam(r, "id")); err != nil { jsonErr(w, 500, err.Error()); return }
	json200(w, map[string]string{"status": "deleted"})
}

// ── #57 Compose editor validation ──

func ComposeValidate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil { jsonErr(w, 400, "cannot read body"); return }
	var raw any
	// Try to parse the body as JSON with a "spec" field first
	var req struct{ Spec string `json:"spec"` }
	if json.Unmarshal(body, &req) == nil && req.Spec != "" {
		body = []byte(req.Spec)
	}
	if err := yaml.Unmarshal(body, &raw); err != nil {
		json200(w, map[string]any{"valid": false, "error": err.Error()}); return
	}
	json200(w, map[string]any{"valid": true})
}

// ── #58 Streaming logs ──
// ServiceLogs is already defined above — we replace it to support follow=true SSE streaming

// ── #61 Auto-deploy on image push (enhanced WebhookTrigger) ──
// WebhookTrigger is already defined above — we replace it to parse DockerHub/generic webhook body
