// Levier - model provider catalog.
//
// Provider records are metadata plus a secret reference. Secret values are
// resolved by the control plane at dispatch time and never stored in the
// catalog or returned by the API.
//
// Copyright 2026 The Levier Authors.
package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fluffyTighnari/levier/internal/id"
)

var (
	// ErrModelProviderNotFound reports an unknown provider name.
	ErrModelProviderNotFound = errors.New("register: model provider not found")
	// ErrModelProviderInvalid reports an invalid provider definition.
	ErrModelProviderInvalid = errors.New("register: invalid model provider")
)

// ModelProviderInput is one provider definition without a secret value.
type ModelProviderInput struct {
	Name         string
	Adapter      string
	BaseURL      string
	DefaultModel string
	Models       []string
	SecretRef    string
	Enabled      bool
	Default      bool
}

// ModelProvider is the admin-visible provider projection.
type ModelProvider struct {
	Name             string    `json:"name"`
	Adapter          string    `json:"adapter"`
	BaseURL          string    `json:"base_url"`
	DefaultModel     string    `json:"default_model"`
	Models           []string  `json:"models"`
	SecretRef        string    `json:"secret_ref"`
	SecretConfigured bool      `json:"secret_configured"`
	Enabled          bool      `json:"enabled"`
	Default          bool      `json:"default"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ModelProfile is the resolved profile handed to a runtime.
type ModelProfile struct {
	Provider string
	Adapter  string
	Model    string
	BaseURL  string
	APIKey   string
	Params   map[string]string
}

const modelProviderColumns = `
	provider_id, name, adapter, base_url, default_model, models_json,
	secret_ref, enabled, is_default, created_at, updated_at`

// UpsertModelProvider creates or updates one provider metadata record.
func (r *Registry) UpsertModelProvider(ctx context.Context, input ModelProviderInput) (ModelProvider, error) {
	if r == nil || r.pool == nil {
		return ModelProvider{}, fmt.Errorf("register: postgres pool is nil")
	}
	name := strings.TrimSpace(input.Name)
	adapter := strings.TrimSpace(input.Adapter)
	if name == "" {
		return ModelProvider{}, fmt.Errorf("%w: name is required", ErrModelProviderInvalid)
	}
	switch adapter {
	case "openai_compatible", "anthropic":
	default:
		return ModelProvider{}, fmt.Errorf("%w: adapter must be openai_compatible or anthropic", ErrModelProviderInvalid)
	}
	models := cleanModels(input.Models)
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return ModelProvider{}, fmt.Errorf("%w: encode models", ErrModelProviderInvalid)
	}
	secretRef := strings.TrimSpace(input.SecretRef)
	if secretRef != "" && strings.ContainsAny(secretRef, " \t\r\n") {
		return ModelProvider{}, fmt.Errorf("%w: secret_ref must not contain whitespace", ErrModelProviderInvalid)
	}
	lowerRef := strings.ToLower(secretRef)
	if strings.HasPrefix(lowerRef, "sk-") || strings.HasPrefix(lowerRef, "mw_") || strings.HasPrefix(lowerRef, "bearer ") {
		return ModelProvider{}, fmt.Errorf("%w: secret_ref must be a name such as deepseek_api_key, not an API key; store the key in LEVIER_SECRET_* or LEVIER_SECRETS_FILE", ErrModelProviderInvalid)
	}
	if input.Default && !input.Enabled {
		return ModelProvider{}, fmt.Errorf("%w: the default provider must be enabled", ErrModelProviderInvalid)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ModelProvider{}, fmt.Errorf("register: begin model provider: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if input.Default {
		if _, err := tx.Exec(ctx, "UPDATE model_providers SET is_default = FALSE, updated_at = now()"); err != nil {
			return ModelProvider{}, fmt.Errorf("register: clear default model provider: %w", err)
		}
	}
	providerID := "mdl_" + id.New()
	row := tx.QueryRow(ctx, `
		INSERT INTO model_providers
			(provider_id, name, adapter, base_url, default_model, models_json,
			 secret_ref, enabled, is_default, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
		ON CONFLICT (name) DO UPDATE SET
			adapter = EXCLUDED.adapter,
			base_url = EXCLUDED.base_url,
			default_model = EXCLUDED.default_model,
			models_json = EXCLUDED.models_json,
			secret_ref = EXCLUDED.secret_ref,
			enabled = EXCLUDED.enabled,
			is_default = EXCLUDED.is_default,
			updated_at = now()
		RETURNING `+modelProviderColumns,
		providerID, name, adapter, strings.TrimSpace(input.BaseURL),
		strings.TrimSpace(input.DefaultModel), string(modelsJSON),
		secretRef, input.Enabled, input.Default,
	)
	provider, err := scanModelProvider(row)
	if err != nil {
		return ModelProvider{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ModelProvider{}, fmt.Errorf("register: commit model provider: %w", err)
	}
	provider.SecretConfigured = r.secretConfigured(ctx, provider.SecretRef)
	return provider, nil
}

// ListModelProviders returns all provider metadata without secret values.
func (r *Registry) ListModelProviders(ctx context.Context) ([]ModelProvider, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("register: postgres pool is nil")
	}
	rows, err := r.pool.Query(ctx, "SELECT "+modelProviderColumns+" FROM model_providers ORDER BY is_default DESC, name")
	if err != nil {
		return nil, fmt.Errorf("register: list model providers: %w", err)
	}
	defer rows.Close()
	providers := make([]ModelProvider, 0)
	for rows.Next() {
		provider, err := scanModelProvider(rows)
		if err != nil {
			return nil, err
		}
		provider.SecretConfigured = r.secretConfigured(ctx, provider.SecretRef)
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("register: iterate model providers: %w", err)
	}
	return providers, nil
}

// GetModelProvider returns one provider by name.
func (r *Registry) GetModelProvider(ctx context.Context, name string) (ModelProvider, error) {
	if r == nil || r.pool == nil {
		return ModelProvider{}, fmt.Errorf("register: postgres pool is nil")
	}
	provider, err := scanModelProvider(r.pool.QueryRow(ctx,
		"SELECT "+modelProviderColumns+" FROM model_providers WHERE name = $1",
		strings.TrimSpace(name),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ModelProvider{}, ErrModelProviderNotFound
	}
	if err != nil {
		return ModelProvider{}, err
	}
	provider.SecretConfigured = r.secretConfigured(ctx, provider.SecretRef)
	return provider, nil
}

// DeleteModelProvider removes one provider by name.
func (r *Registry) DeleteModelProvider(ctx context.Context, name string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("register: postgres pool is nil")
	}
	tag, err := r.pool.Exec(ctx,
		"DELETE FROM model_providers WHERE name = $1", strings.TrimSpace(name))
	if err != nil {
		return fmt.Errorf("register: delete model provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrModelProviderNotFound
	}
	return nil
}

// CheckModelProvider validates that a provider exists and its secret can be resolved.
func (r *Registry) CheckModelProvider(ctx context.Context, name string) (ModelProvider, error) {
	provider, err := r.GetModelProvider(ctx, name)
	if err != nil {
		return ModelProvider{}, err
	}
	if !provider.Enabled {
		return ModelProvider{}, fmt.Errorf("%w: provider %q is disabled", ErrModelProviderInvalid, provider.Name)
	}
	if provider.SecretRef != "" {
		if _, err := r.secretResolver().Resolve(ctx, provider.SecretRef); err != nil {
			return ModelProvider{}, fmt.Errorf("%w: provider %q secret is not configured; set LEVIER_SECRET_%s or LEVIER_SECRETS_FILE", ErrModelProviderInvalid, provider.Name, strings.ToUpper(strings.ReplaceAll(provider.SecretRef, "-", "_")))
		}
	}
	return provider, nil
}

// PutModelProviderSecret stores the provider's API key in the managed store.
func (r *Registry) PutModelProviderSecret(ctx context.Context, name, apiKey string) (ModelProvider, error) {
	if r == nil || r.secretWriter == nil {
		return ModelProvider{}, fmt.Errorf("%w: managed provider secrets are not enabled; set LEVIER_SECRETS_WRITE_FILE", ErrModelProviderInvalid)
	}
	provider, err := r.GetModelProvider(ctx, name)
	if err != nil {
		return ModelProvider{}, err
	}
	if strings.TrimSpace(provider.SecretRef) == "" {
		return ModelProvider{}, fmt.Errorf("%w: provider %q has no secret_ref", ErrModelProviderInvalid, provider.Name)
	}
	if err := r.secretWriter.PutSecret(ctx, provider.SecretRef, apiKey); err != nil {
		return ModelProvider{}, fmt.Errorf("%w: store provider secret: %v", ErrModelProviderInvalid, err)
	}
	return r.GetModelProvider(ctx, name)
}

// ResolveModelProfile turns a requested provider/model into a runtime profile.
// An empty provider name selects the configured default provider.
func (r *Registry) ResolveModelProfile(ctx context.Context, name, model string, params map[string]string) (ModelProfile, error) {
	if strings.TrimSpace(name) == "" {
		return r.resolveDefault(ctx, model, params)
	}
	provider, err := r.GetModelProvider(ctx, name)
	if err != nil {
		return ModelProfile{}, err
	}
	return r.profileFromProvider(ctx, provider, model, params)
}

// ResolveDefaultModelProfile returns the configured default provider, or the
// first enabled provider when no explicit default exists.
func (r *Registry) ResolveDefaultModelProfile(ctx context.Context, params map[string]string) (ModelProfile, error) {
	return r.resolveDefault(ctx, "", params)
}

func (r *Registry) resolveDefault(ctx context.Context, model string, params map[string]string) (ModelProfile, error) {
	if r == nil || r.pool == nil {
		return ModelProfile{}, fmt.Errorf("register: postgres pool is nil")
	}
	provider, err := scanModelProvider(r.pool.QueryRow(ctx,
		"SELECT "+modelProviderColumns+" FROM model_providers WHERE enabled ORDER BY is_default DESC, name LIMIT 1",
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ModelProfile{}, ErrModelProviderNotFound
	}
	if err != nil {
		return ModelProfile{}, err
	}
	return r.profileFromProvider(ctx, provider, model, params)
}

func (r *Registry) profileFromProvider(ctx context.Context, provider ModelProvider, model string, params map[string]string) (ModelProfile, error) {
	if !provider.Enabled {
		return ModelProfile{}, fmt.Errorf("%w: provider %q is disabled", ErrModelProviderInvalid, provider.Name)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = provider.DefaultModel
	}
	if model == "" {
		return ModelProfile{}, fmt.Errorf("%w: provider %q has no default model", ErrModelProviderInvalid, provider.Name)
	}
	if len(provider.Models) > 0 && !contains(provider.Models, model) {
		return ModelProfile{}, fmt.Errorf("%w: model %q is not allowed for provider %q", ErrModelProviderInvalid, model, provider.Name)
	}
	apiKey := ""
	var err error
	if provider.SecretRef != "" {
		apiKey, err = r.secretResolver().Resolve(ctx, provider.SecretRef)
		if err != nil {
			return ModelProfile{}, fmt.Errorf("%w: provider %q secret is not configured", ErrModelProviderInvalid, provider.Name)
		}
	}
	adapter := provider.Adapter
	runtimeProvider := adapter
	switch adapter {
	case "openai_compatible":
		runtimeProvider = "openai"
	case "anthropic":
		runtimeProvider = "anthropic"
	}
	return ModelProfile{
		Provider: runtimeProvider,
		Adapter:  adapter,
		Model:    model,
		BaseURL:  provider.BaseURL,
		APIKey:   apiKey,
		Params:   params,
	}, nil
}

func scanModelProvider(row pgx.Row) (ModelProvider, error) {
	var provider ModelProvider
	var providerID string
	var modelsJSON string
	if err := row.Scan(
		&providerID,
		&provider.Name,
		&provider.Adapter,
		&provider.BaseURL,
		&provider.DefaultModel,
		&modelsJSON,
		&provider.SecretRef,
		&provider.Enabled,
		&provider.Default,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	); err != nil {
		return ModelProvider{}, fmt.Errorf("register: scan model provider: %w", err)
	}
	// provider_id is intentionally not exposed.
	_ = providerID
	if err := json.Unmarshal([]byte(modelsJSON), &provider.Models); err != nil {
		return ModelProvider{}, fmt.Errorf("register: parse model provider models: %w", err)
	}
	return provider, nil
}

func cleanModels(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (r *Registry) secretConfigured(ctx context.Context, ref string) bool {
	if ref == "" {
		return true
	}
	if r == nil {
		return false
	}
	_, err := r.secretResolver().Resolve(ctx, ref)
	return err == nil
}
