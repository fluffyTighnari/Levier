// Levier — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"errors"
	"github.com/fluffyTighnari/levier/internal/errs"
	"net/http"
	"strings"

	"github.com/fluffyTighnari/levier/internal/register"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

func (s *Server) handleAdminCreateApp(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	if s.apps == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "app registry is not configured", false)
		return
	}
	var request AppUpsertRequest
	if _, ok := s.readJSONBody(w, r, &request, true); !ok {
		return
	}
	if err := request.Validate(); err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	tenantID := strings.TrimSpace(request.TenantID)
	if tenantID == "" {
		tenantID = runstore.DefaultTenantID
	}
	var app runstore.App
	var key string
	var created bool
	var err error
	if registrar, ok := s.apps.(AppTenantRegistrar); ok {
		app, key, created, err = registrar.RegisterAppInTenant(
			r.Context(), tenantID, request.Name, request.Multiagent,
		)
	} else if tenantID == "default" {
		app, key, created, err = s.apps.RegisterApp(r.Context(), request.Name, request.Multiagent)
	} else {
		writeErrorCode(w, errs.CodeNotImplemented, "tenant-aware app registration is not configured", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "create app failed", true)
		return
	}
	if created && !s.applyAppCatalog(w, r, app.AppID, request) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, CreateAppResponse{App: appView(app), APIKey: key})
}

func (s *Server) handleAdminListApps(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	lister, ok := s.apps.(AppLister)
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "app registry does not support listing", false)
		return
	}
	apps, err := lister.ListApps(r.Context())
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "list apps failed", true)
		return
	}
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	views := make([]AppView, 0, len(apps))
	for _, app := range apps {
		if tenantID != "" && app.TenantID != tenantID {
			continue
		}
		views = append(views, appView(app))
	}
	writeJSON(w, http.StatusOK, AppListResponse{Apps: views})
}

func (s *Server) applyAppCatalog(w http.ResponseWriter, r *http.Request, appID string, request AppUpsertRequest) bool {
	catalog, ok := s.apps.(CatalogRegistry)
	if !ok {
		return true
	}
	if request.Tools != nil {
		tools := make([]register.ToolInput, 0, len(request.Tools))
		for _, tool := range request.Tools {
			schema := string(tool.Schema)
			if schema == "" {
				schema = "{}"
			}
			tools = append(tools, register.ToolInput{
				Name:        tool.Name,
				Description: tool.Description,
				SchemaJSON:  schema,
				TimeoutMS:   int(tool.TimeoutMS),
			})
		}
		if _, err := catalog.ReplaceLocalTools(r.Context(), appID, tools); err != nil {
			writeErrorCode(w, errs.CodeBadTool, err.Error(), false)
			return false
		}
	}
	if request.MCPRefs != nil {
		if err := catalog.SetAppMCPRefs(r.Context(), appID, request.MCPRefs); err != nil {
			writeErrorCode(w, errs.CodeBadMCPRef, err.Error(), false)
			return false
		}
	}
	return true
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromRequest(r)
	if err != nil {
		writeAppAuthError(w, err)
		return
	}
	var request AppUpsertRequest
	if _, ok := s.readJSONBody(w, r, &request, true); !ok {
		return
	}
	if err := request.Validate(); err != nil {
		writeErrorCode(w, errs.CodeBadRequest, err.Error(), false)
		return
	}
	if tenantID := strings.TrimSpace(request.TenantID); tenantID != "" && tenantID != app.TenantID {
		writeErrorCode(w, errs.CodeBadRequest, "tenant_id cannot be changed", false)
		return
	}
	updated, err := s.apps.UpdateApp(r.Context(), app.AppID, request.Name, request.Multiagent)
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeErrorCode(w, errs.CodeAppNotFound, "app not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeAppNameConflict, "app name is already in use", false)
		return
	}
	if !s.applyAppCatalog(w, r, app.AppID, request) {
		return
	}
	writeJSON(w, http.StatusOK, appView(updated))
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("app_id")
	if !s.isAdmin(r) {
		app, err := s.appFromRequest(r)
		if err != nil {
			writeAppAuthError(w, err)
			return
		}
		if app.AppID != appID {
			writeErrorCode(w, errs.CodeAppNotFound, "app not found", false)
			return
		}
	}
	if s.apps == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "app registry is not configured", false)
		return
	}
	app, err := s.apps.GetApp(r.Context(), appID)
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeErrorCode(w, errs.CodeAppNotFound, "app not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get app failed", true)
		return
	}
	writeJSON(w, http.StatusOK, appView(app))
}

func (s *Server) handleAdminRotateKey(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	if s.apps == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "app registry is not configured", false)
		return
	}
	key, err := s.apps.RotateAPIKey(r.Context(), r.PathValue("app_id"))
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeErrorCode(w, errs.CodeAppNotFound, "app not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "rotate app key failed", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_key": key})
}

func appView(app runstore.App) AppView {
	return AppView{
		AppID:      app.AppID,
		TenantID:   app.TenantID,
		Name:       app.Name,
		Multiagent: app.Multiagent,
		Status:     app.Status,
		CreatedAt:  app.CreatedAt,
		UpdatedAt:  app.UpdatedAt,
	}
}
