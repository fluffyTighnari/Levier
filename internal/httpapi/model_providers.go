// Levier - model provider admin and App catalog API.
//
// Provider records contain metadata and a secret reference only. Secret values
// are resolved by the control plane at dispatch time and never returned here.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/fluffyTighnari/levier/internal/errs"
	"github.com/fluffyTighnari/levier/internal/register"
)

// ModelProviderRegistry is the provider catalog subset used by HTTP.
type ModelProviderRegistry interface {
	UpsertModelProvider(ctx context.Context, input register.ModelProviderInput) (register.ModelProvider, error)
	ListModelProviders(ctx context.Context) ([]register.ModelProvider, error)
	GetModelProvider(ctx context.Context, name string) (register.ModelProvider, error)
	DeleteModelProvider(ctx context.Context, name string) error
	CheckModelProvider(ctx context.Context, name string) (register.ModelProvider, error)
}

// ModelProviderUpsertRequest is the admin POST body.
type ModelProviderUpsertRequest struct {
	Name         string   `json:"name"`
	Adapter      string   `json:"adapter"`
	BaseURL      string   `json:"base_url"`
	DefaultModel string   `json:"default_model"`
	Models       []string `json:"models,omitempty"`
	SecretRef    string   `json:"secret_ref"`
	Enabled      *bool    `json:"enabled,omitempty"`
	Default      *bool    `json:"default,omitempty"`
}

// ModelProviderView is the admin-visible provider projection.
type ModelProviderView struct {
	Name             string   `json:"name"`
	Adapter          string   `json:"adapter"`
	BaseURL          string   `json:"base_url"`
	DefaultModel     string   `json:"default_model"`
	Models           []string `json:"models"`
	SecretRef        string   `json:"secret_ref"`
	SecretConfigured bool     `json:"secret_configured"`
	Enabled          bool     `json:"enabled"`
	Default          bool     `json:"default"`
}

// ModelProviderPublicView omits secret references and configuration state.
type ModelProviderPublicView struct {
	Name         string   `json:"name"`
	Adapter      string   `json:"adapter"`
	BaseURL      string   `json:"base_url"`
	DefaultModel string   `json:"default_model"`
	Models       []string `json:"models"`
	Default      bool     `json:"default"`
}

// ModelProviderListResponse is the list body.
type ModelProviderListResponse struct {
	Providers []ModelProviderView `json:"providers"`
}

// PublicModelProviderListResponse is the App-facing list body.
type PublicModelProviderListResponse struct {
	Providers []ModelProviderPublicView `json:"providers"`
}

func (s *Server) modelProviders() (ModelProviderRegistry, bool) {
	registry, ok := s.apps.(ModelProviderRegistry)
	return registry, ok
}

func modelProviderView(provider register.ModelProvider) ModelProviderView {
	models := provider.Models
	if models == nil {
		models = []string{}
	}
	return ModelProviderView{
		Name:             provider.Name,
		Adapter:          provider.Adapter,
		BaseURL:          provider.BaseURL,
		DefaultModel:     provider.DefaultModel,
		Models:           models,
		SecretRef:        provider.SecretRef,
		SecretConfigured: provider.SecretConfigured,
		Enabled:          provider.Enabled,
		Default:          provider.Default,
	}
}

func publicModelProviderView(provider register.ModelProvider) ModelProviderPublicView {
	models := provider.Models
	if models == nil {
		models = []string{}
	}
	return ModelProviderPublicView{
		Name:         provider.Name,
		Adapter:      provider.Adapter,
		BaseURL:      provider.BaseURL,
		DefaultModel: provider.DefaultModel,
		Models:       models,
		Default:      provider.Default,
	}
}

func writeModelProviderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, register.ErrModelProviderNotFound):
		writeErrorCode(w, errs.CodeModelProviderNotFound, err.Error(), false)
	case errors.Is(err, register.ErrModelProviderInvalid):
		writeErrorCode(w, errs.CodeBadModelProvider, err.Error(), false)
	default:
		writeErrorCode(w, errs.CodeInternal, "model provider operation failed", true)
	}
}

func (s *Server) handleAdminListModelProviders(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	registry, ok := s.modelProviders()
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "model provider registry is not configured", false)
		return
	}
	providers, err := registry.ListModelProviders(r.Context())
	if err != nil {
		writeModelProviderError(w, err)
		return
	}
	views := make([]ModelProviderView, 0, len(providers))
	for _, provider := range providers {
		views = append(views, modelProviderView(provider))
	}
	writeJSON(w, http.StatusOK, ModelProviderListResponse{Providers: views})
}

func (s *Server) handleAdminGetModelProvider(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	registry, ok := s.modelProviders()
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "model provider registry is not configured", false)
		return
	}
	provider, err := registry.GetModelProvider(r.Context(), r.PathValue("name"))
	if err != nil {
		writeModelProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, modelProviderView(provider))
}

func (s *Server) handleAdminUpsertModelProvider(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	registry, ok := s.modelProviders()
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "model provider registry is not configured", false)
		return
	}
	var request ModelProviderUpsertRequest
	if _, ok := s.readJSONBody(w, r, &request, true); !ok {
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	defaultProvider := false
	if request.Default != nil {
		defaultProvider = *request.Default
	}
	provider, err := registry.UpsertModelProvider(r.Context(), register.ModelProviderInput{
		Name:         strings.TrimSpace(request.Name),
		Adapter:      strings.TrimSpace(request.Adapter),
		BaseURL:      strings.TrimSpace(request.BaseURL),
		DefaultModel: strings.TrimSpace(request.DefaultModel),
		Models:       request.Models,
		SecretRef:    strings.TrimSpace(request.SecretRef),
		Enabled:      enabled,
		Default:      defaultProvider,
	})
	if err != nil {
		writeModelProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, modelProviderView(provider))
}

func (s *Server) handleAdminDeleteModelProvider(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	registry, ok := s.modelProviders()
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "model provider registry is not configured", false)
		return
	}
	if err := registry.DeleteModelProvider(r.Context(), r.PathValue("name")); err != nil {
		writeModelProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleAdminCheckModelProvider(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeErrorCode(w, errs.CodeUnauthorized, "admin token is required", false)
		return
	}
	registry, ok := s.modelProviders()
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "model provider registry is not configured", false)
		return
	}
	provider, err := registry.CheckModelProvider(r.Context(), r.PathValue("name"))
	if err != nil {
		writeModelProviderError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, modelProviderView(provider))
}

func (s *Server) handlePublicListModelProviders(w http.ResponseWriter, r *http.Request) {
	if !s.authDisabled() && !s.isAdmin(r) {
		if _, err := s.appFromRequest(r); err != nil {
			writeAppAuthError(w, err)
			return
		}
	}
	registry, ok := s.modelProviders()
	if !ok {
		writeErrorCode(w, errs.CodeNotImplemented, "model provider registry is not configured", false)
		return
	}
	providers, err := registry.ListModelProviders(r.Context())
	if err != nil {
		writeModelProviderError(w, err)
		return
	}
	views := make([]ModelProviderPublicView, 0, len(providers))
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		views = append(views, publicModelProviderView(provider))
	}
	writeJSON(w, http.StatusOK, PublicModelProviderListResponse{Providers: views})
}
