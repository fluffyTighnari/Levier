// Levier — public service self-description.
//
// Copyright 2026 The Levier Authors.
package selfdesc

import (
	"runtime"
	"time"
)

const (
	// MetaVersion is the public self-description schema version.
	MetaVersion = 1
	// ServiceName is the public service identifier.
	ServiceName = "levier"

	// DefaultMaxBodyBytes is the default HTTP request body limit.
	DefaultMaxBodyBytes int64 = 1 << 20
	// DefaultRunTimeoutMS is the default run timeout.
	DefaultRunTimeoutMS = 1_800_000
	// DefaultToolTimeoutMS is the default tool timeout.
	DefaultToolTimeoutMS = 60_000
	// DefaultMaxToolTimeoutMS is the maximum accepted tool timeout.
	DefaultMaxToolTimeoutMS = 240_000
	// DefaultWSTokenTTL is the default WebSocket token lifetime.
	DefaultWSTokenTTL = 10 * time.Minute
)

// Document is the stable public description returned by GET /meta.
type Document struct {
	MetaVersion  int          `json:"meta_version"`
	Service      string       `json:"service"`
	Build        Build        `json:"build"`
	API          API          `json:"api"`
	Capabilities Capabilities `json:"capabilities"`
	Limits       Limits       `json:"limits"`
	Auth         Auth         `json:"auth"`
	Endpoints    Endpoints    `json:"endpoints"`
}

// Build describes the running Levier binary.
type Build struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Built    string `json:"built"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
}

// API describes the public HTTP and WebSocket contract versions.
type API struct {
	HTTPVersion  string `json:"http_version"`
	HTTPBasePath string `json:"http_base_path"`
	WSPath       string `json:"ws_path"`
	WSProtocol   int    `json:"ws_protocol"`
}

// Capabilities lists implemented capabilities. It intentionally does not
// advertise sandbox/longterm runtimes or session features that are not part
// of the current MVP.
type Capabilities struct {
	RuntimeTypes  []string `json:"runtime_types"`
	ToolScopes    []string `json:"tool_scopes"`
	MCPTransports []string `json:"mcp_transports"`
	Features      []string `json:"features"`
}

// Limits exposes effective public request limits, never secret values.
type Limits struct {
	MaxBodyBytes         int64 `json:"max_body_bytes"`
	DefaultRunTimeoutMS  int   `json:"default_run_timeout_ms"`
	DefaultToolTimeoutMS int   `json:"default_tool_timeout_ms"`
	MaxToolTimeoutMS     int   `json:"max_tool_timeout_ms"`
	WSTokenTTLSeconds    int64 `json:"ws_token_ttl_seconds"`
}

// Auth describes public authentication schemes, not credentials.
type Auth struct {
	App       AuthScheme `json:"app"`
	Admin     AuthScheme `json:"admin"`
	WebSocket AuthScheme `json:"websocket"`
}

// AuthScheme describes one authentication mechanism.
type AuthScheme struct {
	Scheme    string `json:"scheme"`
	Header    string `json:"header,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

// Endpoints lists stable public HTTP paths. Path placeholders are literal.
type Endpoints struct {
	Health    string `json:"health"`
	Ready     string `json:"ready"`
	Metrics   string `json:"metrics"`
	Apps      string `json:"apps"`
	App       string `json:"app"`
	Runs      string `json:"runs"`
	Run       string `json:"run"`
	RunEvents string `json:"run_events"`
	RunCancel string `json:"run_cancel"`
	WSToken   string `json:"ws_token"`
	WebSocket string `json:"websocket"`
	AdminApps string `json:"admin_apps"`
	AdminMCP  string `json:"admin_mcp"`
}

// Options supplies build and runtime values used to construct a Document.
type Options struct {
	Version              string
	Commit               string
	Built                string
	MaxBodyBytes         int64
	DefaultRunTimeoutMS  int
	DefaultToolTimeoutMS int
	MaxToolTimeoutMS     int
	WSTokenTTL           time.Duration
}

// New constructs the public self-description document.
func New(opts Options) Document {
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	commit := opts.Commit
	if commit == "" {
		commit = "unknown"
	}
	built := opts.Built
	if built == "" {
		built = "unknown"
	}
	maxBody := opts.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}
	runTimeout := opts.DefaultRunTimeoutMS
	if runTimeout <= 0 {
		runTimeout = DefaultRunTimeoutMS
	}
	toolTimeout := opts.DefaultToolTimeoutMS
	if toolTimeout <= 0 {
		toolTimeout = DefaultToolTimeoutMS
	}
	maxToolTimeout := opts.MaxToolTimeoutMS
	if maxToolTimeout <= 0 {
		maxToolTimeout = DefaultMaxToolTimeoutMS
	}
	wsTTL := opts.WSTokenTTL
	if wsTTL <= 0 {
		wsTTL = DefaultWSTokenTTL
	}

	return Document{
		MetaVersion: MetaVersion,
		Service:     ServiceName,
		Build: Build{
			Version:  version,
			Commit:   commit,
			Built:    built,
			Go:       runtime.Version(),
			Platform: runtime.GOOS + "/" + runtime.GOARCH,
		},
		API: API{
			HTTPVersion:  "v1",
			HTTPBasePath: "/api/v1",
			WSPath:       "/api/v1/ws",
			WSProtocol:   1,
		},
		Capabilities: Capabilities{
			RuntimeTypes:  []string{"normal"},
			ToolScopes:    []string{"local", "global"},
			MCPTransports: []string{"stdio", "sse", "streamable_http"},
			Features: []string{
				"run_submit",
				"run_cancel",
				"event_replay",
				"app_registration",
				"mcp_management",
				"websocket",
				"local_tools",
				"global_tools",
				"mcp_tools",
			},
		},
		Limits: Limits{
			MaxBodyBytes:         maxBody,
			DefaultRunTimeoutMS:  runTimeout,
			DefaultToolTimeoutMS: toolTimeout,
			MaxToolTimeoutMS:     maxToolTimeout,
			WSTokenTTLSeconds:    int64(wsTTL / time.Second),
		},
		Auth: Auth{
			App:       AuthScheme{Scheme: "bearer", Header: "Authorization"},
			Admin:     AuthScheme{Scheme: "bearer", Header: "Authorization"},
			WebSocket: AuthScheme{Scheme: "query", Parameter: "token"},
		},
		Endpoints: Endpoints{
			Health:    "/healthz",
			Ready:     "/readyz",
			Metrics:   "/metrics",
			Apps:      "/api/v1/apps",
			App:       "/api/v1/apps/{app_id}",
			Runs:      "/api/v1/runs",
			Run:       "/api/v1/runs/{run_id}",
			RunEvents: "/api/v1/runs/{run_id}/events",
			RunCancel: "/api/v1/runs/{run_id}/cancel",
			WSToken:   "/api/v1/ws-token",
			WebSocket: "/api/v1/ws",
			AdminApps: "/admin/apps",
			AdminMCP:  "/admin/mcp",
		},
	}
}

// Default returns a document using all defaults.
func Default() Document {
	return New(Options{})
}
