// Package selfcheck runs ordered, hop-by-hop diagnostics.
//
// Copyright 2026 The Levier Authors.
package selfcheck

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fluffyTighnari/levier/internal/ops/selfdesc"
	statuspkg "github.com/fluffyTighnari/levier/internal/ops/status"
	"github.com/fluffyTighnari/levier/internal/platform/config"
	"github.com/fluffyTighnari/levier/internal/platform/db"
	"github.com/fluffyTighnari/levier/internal/runtime/daemoncheck"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/net/websocket"
)

const (
	// StatusOK means the hop passed.
	StatusOK = "ok"
	// StatusFail means the hop failed.
	StatusFail = "fail"
	// StatusSkip means the hop could not run or was not requested.
	StatusSkip = "skip"
)

// Hop is one ordered self-check result.
type Hop struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	Fix        string `json:"fix,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// Report is one complete self-check run.
type Report struct {
	OK         bool   `json:"ok"`
	Server     string `json:"server"`
	Deep       bool   `json:"deep"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Hops       []Hop  `json:"hops"`
}

// Options controls one self-check run.
type Options struct {
	ConfigPath  string
	Server      string
	GRPCAddr    string
	AdminToken  string
	Credentials string
	DaemonID    string
	DaemonToken string
	RuntimeName string
	MCPSocket   string
	Timeout     time.Duration
	Deep        bool
}

type runner struct {
	opts       Options
	client     *http.Client
	cfg        config.Config
	haveCfg    bool
	status     statuspkg.Snapshot
	haveStatus bool
	pool       *dbPool
}

// dbPool is the small subset needed to keep the database open for two hops.
type dbPool struct {
	pool  *pgxpool.Pool
	close func()
}

// Run executes the self-check in dependency order.
func Run(opts Options) Report {
	opts = withDefaults(opts)
	started := time.Now().UTC()
	r := &runner{
		opts:   opts,
		client: &http.Client{Timeout: opts.Timeout},
	}
	report := Report{
		Server:    opts.Server,
		Deep:      opts.Deep,
		StartedAt: started.Format(time.RFC3339),
		Hops:      make([]Hop, 0, 12),
	}
	defer func() {
		if r.pool != nil && r.pool.close != nil {
			r.pool.close()
		}
	}()

	r.add(&report, "config", r.checkConfig())
	r.add(&report, "http /healthz", r.checkHTTP("/healthz", "health"))
	r.add(&report, "database", r.checkDatabase())
	r.add(&report, "migrations", r.checkMigrations())
	r.add(&report, "http /readyz", r.checkHTTP("/readyz", "ready"))
	r.add(&report, "service /meta", r.checkMeta())
	r.add(&report, "service /status", r.checkStatus())
	r.add(&report, "admin auth", r.checkAdmin())
	r.add(&report, "daemon registered", r.checkDaemon())
	r.add(&report, "app credentials", r.checkApp())

	if opts.Deep {
		r.add(&report, "deep daemoncheck", r.checkDeepDaemon())
		r.add(&report, "deep app websocket", r.checkDeepApp())
		r.add(&report, "deep mcp", r.checkDeepMCP())
	}

	report.OK = true
	for _, hop := range report.Hops {
		if hop.Status == StatusFail {
			report.OK = false
			break
		}
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return report
}

func withDefaults(opts Options) Options {
	if opts.ConfigPath == "" {
		opts.ConfigPath = envOr("LEVIER_CONFIG", "levier.example.yaml")
	}
	if opts.Server == "" {
		opts.Server = envOr("LEVIER_SERVER", "http://127.0.0.1:8080")
	}
	opts.Server = strings.TrimRight(opts.Server, "/")
	if opts.GRPCAddr == "" {
		opts.GRPCAddr = envOr("LEVIER_GRPC", "127.0.0.1:9090")
	}
	if opts.AdminToken == "" {
		opts.AdminToken = os.Getenv("LEVIER_ADMIN_TOKEN")
	}
	if opts.Credentials == "" {
		opts.Credentials = os.Getenv("LEVIER_CREDENTIALS")
	}
	if opts.DaemonID == "" {
		opts.DaemonID = envOr("LEVIER_DAEMON_ID", "daemon-local-1")
	}
	if opts.DaemonToken == "" {
		opts.DaemonToken = os.Getenv("LEVIER_DAEMON_TOKEN")
	}
	if opts.MCPSocket == "" {
		opts.MCPSocket = os.Getenv("LEVIER_MCP_SOCKET")
	}
	if opts.RuntimeName == "" {
		opts.RuntimeName = "echo1"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	return opts
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func (r *runner) add(report *Report, name string, hop Hop) {
	hop.Name = name
	report.Hops = append(report.Hops, hop)
}

func (r *runner) checkConfig() Hop {
	started := time.Now()
	cfg, err := config.Load(r.opts.ConfigPath)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --config path or LEVIER_CONFIG"}
	}
	r.cfg = cfg
	r.haveCfg = true
	if r.opts.AdminToken == "" {
		r.opts.AdminToken = cfg.HTTP.AdminToken
	}
	return Hop{Status: StatusOK, Message: r.opts.ConfigPath, DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkHTTP(path, name string) Hop {
	started := time.Now()
	code, _, err := r.get(path, "")
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "start levier serve or check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	if code != http.StatusOK {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("status %d", code), Fix: "check " + name + " and server logs", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: "200", DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkDatabase() Hop {
	started := time.Now()
	if !r.haveCfg {
		return Hop{Status: StatusSkip, Message: "config not loaded", DurationMS: time.Since(started).Milliseconds()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.Timeout)
	defer cancel()
	pool, err := db.Open(ctx, db.Config{
		URL:            r.cfg.Database.URL,
		MaxConns:       r.cfg.Database.MaxConns,
		MinConns:       r.cfg.Database.MinConns,
		ConnectTimeout: r.cfg.Database.ConnectTimeout.Std(),
		HealthTimeout:  r.cfg.Database.HealthTimeout.Std(),
	})
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check PostgreSQL and LEVIER_DATABASE_URL", DurationMS: time.Since(started).Milliseconds()}
	}
	r.pool = &dbPool{pool: pool, close: pool.Close}
	if err := db.Health(ctx, pool, r.cfg.Database.HealthTimeout.Std()); err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check PostgreSQL and /readyz", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: "connected", DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkMigrations() Hop {
	started := time.Now()
	if r.pool == nil {
		return Hop{Status: StatusSkip, Message: "database unavailable", DurationMS: time.Since(started).Milliseconds()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.Timeout)
	defer cancel()
	state, err := db.Status(ctx, r.pool.poolRef())
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "run levier migrate up", DurationMS: time.Since(started).Milliseconds()}
	}
	if len(state.Pending) > 0 {
		return Hop{Status: StatusFail,
			Message:    fmt.Sprintf("current %d, pending %d", state.Current, len(state.Pending)),
			Fix:        "run levier migrate up",
			DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: fmt.Sprintf("version %d", state.Current), DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkMeta() Hop {
	started := time.Now()
	code, body, err := r.get("/meta", "")
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "upgrade server or check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	if code != http.StatusOK {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("status %d", code), Fix: "upgrade server or check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	var doc selfdesc.Document
	if err := json.Unmarshal(body, &doc); err != nil {
		return Hop{Status: StatusFail, Message: "invalid /meta JSON: " + err.Error(), Fix: "upgrade server", DurationMS: time.Since(started).Milliseconds()}
	}
	if doc.MetaVersion != selfdesc.MetaVersion || doc.Service != selfdesc.ServiceName {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("service=%q meta_version=%d", doc.Service, doc.MetaVersion), Fix: "upgrade server", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: fmt.Sprintf("v%d", doc.MetaVersion), DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkStatus() Hop {
	started := time.Now()
	code, body, err := r.get("/status", "")
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "upgrade server or check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	if code != http.StatusOK {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("status %d", code), Fix: "upgrade server or check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	var snapshot statuspkg.Snapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return Hop{Status: StatusFail, Message: "invalid /status JSON: " + err.Error(), Fix: "upgrade server", DurationMS: time.Since(started).Milliseconds()}
	}
	if snapshot.StatusVersion != statuspkg.Version || snapshot.Service != statuspkg.ServiceName {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("service=%q status_version=%d", snapshot.Service, snapshot.StatusVersion), Fix: "upgrade server", DurationMS: time.Since(started).Milliseconds()}
	}
	r.status = snapshot
	r.haveStatus = true
	return Hop{Status: StatusOK, Message: fmt.Sprintf("v%d", snapshot.StatusVersion), DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkAdmin() Hop {
	started := time.Now()
	if r.opts.AdminToken == "" {
		return Hop{Status: StatusSkip, Message: "no admin token", DurationMS: time.Since(started).Milliseconds()}
	}
	code, _, err := r.get("/admin/mcp", r.opts.AdminToken)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	if code != http.StatusOK {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("status %d", code), Fix: "check LEVIER_ADMIN_TOKEN / config", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: "200", DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkDaemon() Hop {
	started := time.Now()
	if !r.haveStatus {
		return Hop{Status: StatusSkip, Message: "status unavailable", DurationMS: time.Since(started).Milliseconds()}
	}
	if r.status.Daemons.Connected == 0 {
		return Hop{Status: StatusFail, Message: "no connected daemon", Fix: "start harness and check /status", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK,
		Message:    fmt.Sprintf("connected=%d healthy=%d", r.status.Daemons.Connected, r.status.Daemons.Healthy),
		DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkApp() Hop {
	started := time.Now()
	if r.opts.Credentials == "" {
		return Hop{Status: StatusSkip, Message: "no credentials", DurationMS: time.Since(started).Milliseconds()}
	}
	creds, err := readAppCredentials(r.opts.Credentials)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --credentials file", DurationMS: time.Since(started).Milliseconds()}
	}
	code, _, err := r.get("/api/v1/apps/"+url.PathEscape(creds.AppID), creds.APIKey)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	if code != http.StatusOK {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("status %d", code), Fix: "check App key or rotate it", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: "ok", DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkDeepDaemon() Hop {
	started := time.Now()
	if r.opts.AdminToken == "" || r.opts.DaemonToken == "" {
		return Hop{Status: StatusSkip, Message: "requires --admin-token and --daemon-token", DurationMS: time.Since(started).Milliseconds()}
	}
	checks, err := daemoncheck.Run(daemoncheck.Config{
		GRPCAddr:    r.opts.GRPCAddr,
		HTTPURL:     r.opts.Server,
		DaemonID:    r.opts.DaemonID,
		Token:       r.opts.DaemonToken,
		AdminToken:  r.opts.AdminToken,
		RuntimeName: r.opts.RuntimeName,
		Timeout:     r.opts.Timeout,
	})
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check daemon token, gRPC address and server logs", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: fmt.Sprintf("checks=%d", len(checks)), DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) checkDeepApp() Hop {
	started := time.Now()
	if r.opts.Credentials == "" {
		return Hop{Status: StatusSkip, Message: "no credentials", DurationMS: time.Since(started).Milliseconds()}
	}
	creds, err := readAppCredentials(r.opts.Credentials)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --credentials file", DurationMS: time.Since(started).Milliseconds()}
	}
	code, body, err := r.post("/api/v1/ws-token", creds.APIKey, nil)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --server", DurationMS: time.Since(started).Milliseconds()}
	}
	if code != http.StatusOK {
		return Hop{Status: StatusFail, Message: fmt.Sprintf("ws-token status %d", code), Fix: "check App key or rotate it", DurationMS: time.Since(started).Milliseconds()}
	}
	var token struct {
		WSURL   string `json:"ws_url"`
		WSToken string `json:"ws_token"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return Hop{Status: StatusFail, Message: "invalid ws-token JSON: " + err.Error(), Fix: "upgrade server", DurationMS: time.Since(started).Milliseconds()}
	}
	target, err := r.websocketURL(token.WSURL, creds.AppID, token.WSToken)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "upgrade server", DurationMS: time.Since(started).Milliseconds()}
	}
	config, err := websocket.NewConfig(target, r.opts.Server)
	if err != nil {
		return Hop{Status: StatusFail, Message: "invalid App WebSocket URL", Fix: "check server URL", DurationMS: time.Since(started).Milliseconds()}
	}
	conn, err := websocket.DialConfig(config)
	if err != nil {
		return Hop{Status: StatusFail, Message: "App WebSocket connection failed", Fix: "check App WebSocket and server URL", DurationMS: time.Since(started).Milliseconds()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(r.opts.Timeout))
	if err := websocket.JSON.Send(conn, map[string]any{"type": "hello", "protocol": 1, "client": "levier-selfcheck"}); err != nil {
		return Hop{Status: StatusFail, Message: "App WebSocket hello failed", Fix: "check App WebSocket", DurationMS: time.Since(started).Milliseconds()}
	}
	if err := websocket.JSON.Send(conn, map[string]any{"type": "ping"}); err != nil {
		return Hop{Status: StatusFail, Message: "App WebSocket ping failed", Fix: "check App WebSocket", DurationMS: time.Since(started).Milliseconds()}
	}
	for {
		var message map[string]any
		if err := websocket.JSON.Receive(conn, &message); err != nil {
			return Hop{Status: StatusFail, Message: "waiting for App WebSocket pong failed", Fix: "check App WebSocket", DurationMS: time.Since(started).Milliseconds()}
		}
		if message["type"] == "pong" {
			return Hop{Status: StatusOK, Message: "hello/pong", DurationMS: time.Since(started).Milliseconds()}
		}
	}
}

func (r *runner) checkDeepMCP() Hop {
	started := time.Now()
	if r.opts.MCPSocket == "" {
		return Hop{Status: StatusSkip, Message: "no --mcp-socket", DurationMS: time.Since(started).Milliseconds()}
	}
	conn, err := net.DialTimeout("unix", r.opts.MCPSocket, r.opts.Timeout)
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check --mcp-socket", DurationMS: time.Since(started).Milliseconds()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(r.opts.Timeout))
	request := `{"jsonrpc":"2.0","id":1,"method":"mcp.health","params":{}}` + "\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check MCP runtime", DurationMS: time.Since(started).Milliseconds()}
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return Hop{Status: StatusFail, Message: err.Error(), Fix: "check MCP runtime", DurationMS: time.Since(started).Milliseconds()}
	}
	var response struct {
		Error  any `json:"error"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		return Hop{Status: StatusFail, Message: "invalid mcp.health response: " + err.Error(), Fix: "check MCP runtime", DurationMS: time.Since(started).Milliseconds()}
	}
	if response.Error != nil || response.Result.Status != "ok" {
		return Hop{Status: StatusFail, Message: "mcp.health not ok", Fix: "check MCP runtime", DurationMS: time.Since(started).Milliseconds()}
	}
	return Hop{Status: StatusOK, Message: "mcp.health ok", DurationMS: time.Since(started).Milliseconds()}
}

func (r *runner) post(path, token string, payload any) (int, []byte, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, r.opts.Server+path, strings.NewReader(string(body)))
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	responseBody := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, readErr := response.Body.Read(buf)
		responseBody = append(responseBody, buf[:n]...)
		if readErr != nil {
			break
		}
	}
	return response.StatusCode, responseBody, nil
}

func (r *runner) websocketURL(rawPath, appID, token string) (string, error) {
	base, err := url.Parse(r.opts.Server)
	if err != nil {
		return "", err
	}
	path, err := url.Parse(rawPath)
	if err != nil {
		return "", err
	}
	if path.IsAbs() {
		base = path
	} else {
		base.Path = path.Path
	}
	if base.Scheme == "https" {
		base.Scheme = "wss"
	} else {
		base.Scheme = "ws"
	}
	query := base.Query()
	query.Set("app_id", appID)
	query.Set("token", token)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (r *runner) get(path, token string) (int, []byte, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, r.opts.Server+path, nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, readErr := response.Body.Read(buf)
		body = append(body, buf[:n]...)
		if readErr != nil {
			break
		}
	}
	return response.StatusCode, body, nil
}

type appCredentials struct {
	AppID  string `json:"app_id"`
	APIKey string `json:"api_key"`
}

func readAppCredentials(path string) (appCredentials, error) {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return appCredentials{}, fmt.Errorf("read credentials file: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return appCredentials{}, fmt.Errorf("credentials file permissions must be 0600")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return appCredentials{}, fmt.Errorf("read credentials file: %w", err)
	}
	var creds appCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return appCredentials{}, fmt.Errorf("decode credentials file: %w", err)
	}
	if creds.AppID == "" || creds.APIKey == "" {
		return appCredentials{}, fmt.Errorf("credentials file requires app_id and api_key")
	}
	return creds, nil
}

func (p *dbPool) poolRef() *pgxpool.Pool { return p.pool }
