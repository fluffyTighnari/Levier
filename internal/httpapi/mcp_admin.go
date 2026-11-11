// Middleware — admin MCP catalog API.
//
// Catalog entries carry connection descriptors with ${secret:name}
// placeholders; secrets are resolved by mcp_runtime, never here.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/fluffyTighnari/middleware/internal/register"
)

// MCPRegistry is the admin catalog subset used by the MCP routes.
type MCPRegistry interface {
	UpsertMCPServer(ctx context.Context, input register.MCPServerInput) error
	ListMCPServers(ctx context.Context) ([]register.MCPServerInfo, error)
	DeleteMCPServer(ctx context.Context, name string, force bool) error
}

// MCPServerUpsertRequest is the POST /admin/mcp body.
type MCPServerUpsertRequest struct {
	Name         string          `json:"name"`
	Transport    string          `json:"transport"`
	Config       json.RawMessage `json:"config"`
	AllowedTools []string        `json:"allowed_tools,omitempty"`
}

// MCPServerView is the secret-free catalog view.
type MCPServerView struct {
	Name         string   `json:"name"`
	Transport    string   `json:"transport"`
	AllowedTools []string `json:"allowed_tools"`
	SecretRefs   []string `json:"secret_refs"`
}

// MCPListResponse is the GET /admin/mcp body.
type MCPListResponse struct {
	Servers []MCPServerView `json:"servers"`
}

func (s *Server) mcpRegistry() (MCPRegistry, bool) {
	registry, ok := s.apps.(MCPRegistry)
	return registry, ok
}

func validateMCPServerRequest(request MCPServerUpsertRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("mcp name is required")
	}
	switch request.Transport {
	case "stdio", "sse", "streamable_http":
	default:
		return errors.New("transport must be stdio, sse, or streamable_http")
	}
	var config map[string]any
	if len(request.Config) == 0 || json.Unmarshal(request.Config, &config) != nil {
		return errors.New("config must be a JSON object")
	}
	switch request.Transport {
	case "stdio":
		if command, _ := config["command"].(string); strings.TrimSpace(command) == "" {
			return errors.New("stdio config.command is required")
		}
	default:
		if url, _ := config["url"].(string); strings.TrimSpace(url) == "" {
			return errors.New("http transport config.url is required")
		}
	}
	return nil
}

func (s *Server) handleAdminUpsertMCP(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "admin token is required", false)
		return
	}
	registry, ok := s.mcpRegistry()
	if !ok {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "MCP registry is not configured", false)
		return
	}
	var request MCPServerUpsertRequest
	if _, ok := s.readJSONBody(w, r, &request, true); !ok {
		return
	}
	if err := validateMCPServerRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), false)
		return
	}
	var allowedJSON string
	if len(request.AllowedTools) > 0 {
		encoded, err := json.Marshal(request.AllowedTools)
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "allowed_tools is invalid", false)
			return
		}
		allowedJSON = string(encoded)
	}
	if err := registry.UpsertMCPServer(r.Context(), register.MCPServerInput{
		Name:             strings.TrimSpace(request.Name),
		Transport:        request.Transport,
		ConfigJSON:       string(request.Config),
		AllowedToolsJSON: allowedJSON,
	}); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_MCP", err.Error(), false)
		return
	}
	servers, err := registry.ListMCPServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list MCP servers failed", true)
		return
	}
	for _, server := range servers {
		if server.Name == request.Name {
			writeJSON(w, http.StatusOK, mcpView(server))
			return
		}
	}
	writeJSON(w, http.StatusOK, MCPServerView{
		Name:         request.Name,
		Transport:    request.Transport,
		AllowedTools: request.AllowedTools,
	})
}

func (s *Server) handleAdminListMCP(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "admin token is required", false)
		return
	}
	registry, ok := s.mcpRegistry()
	if !ok {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "MCP registry is not configured", false)
		return
	}
	servers, err := registry.ListMCPServers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "list MCP servers failed", true)
		return
	}
	response := MCPListResponse{Servers: make([]MCPServerView, 0, len(servers))}
	for _, server := range servers {
		response.Servers = append(response.Servers, mcpView(server))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAdminDeleteMCP(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "admin token is required", false)
		return
	}
	registry, ok := s.mcpRegistry()
	if !ok {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "MCP registry is not configured", false)
		return
	}
	force := strings.EqualFold(r.URL.Query().Get("force"), "true")
	err := registry.DeleteMCPServer(r.Context(), r.PathValue("name"), force)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("name")})
	case errors.Is(err, register.ErrMCPServerNotFound):
		writeError(w, http.StatusNotFound, "MCP_NOT_FOUND", "MCP server not found", false)
	case errors.Is(err, register.ErrMCPServerInUse):
		writeError(w, http.StatusConflict, "MCP_IN_USE", "MCP server is referenced by an app", false)
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL", "delete MCP server failed", true)
	}
}

func mcpView(server register.MCPServerInfo) MCPServerView {
	view := MCPServerView{
		Name:         server.Name,
		Transport:    server.Transport,
		AllowedTools: server.AllowedTools,
		SecretRefs:   server.SecretRefs,
	}
	if view.AllowedTools == nil {
		view.AllowedTools = []string{}
	}
	if view.SecretRefs == nil {
		view.SecretRefs = []string{}
	}
	return view
}
