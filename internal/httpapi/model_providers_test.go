package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fluffyTighnari/levier/internal/register"
)

type fakeProviderApps struct {
	*fakeApps
	providers map[string]register.ModelProvider
}

func newFakeProviderApps() *fakeProviderApps {
	return &fakeProviderApps{
		fakeApps:  newFakeApps(),
		providers: make(map[string]register.ModelProvider),
	}
}

func (f *fakeProviderApps) UpsertModelProvider(_ context.Context, input register.ModelProviderInput) (register.ModelProvider, error) {
	provider := register.ModelProvider{
		Name:             input.Name,
		Adapter:          input.Adapter,
		BaseURL:          input.BaseURL,
		DefaultModel:     input.DefaultModel,
		Models:           input.Models,
		SecretRef:        input.SecretRef,
		SecretConfigured: true,
		Enabled:          input.Enabled,
		Default:          input.Default,
	}
	f.providers[input.Name] = provider
	return provider, nil
}
func (f *fakeProviderApps) ListModelProviders(context.Context) ([]register.ModelProvider, error) {
	result := make([]register.ModelProvider, 0, len(f.providers))
	for _, provider := range f.providers {
		result = append(result, provider)
	}
	return result, nil
}
func (f *fakeProviderApps) GetModelProvider(_ context.Context, name string) (register.ModelProvider, error) {
	provider, ok := f.providers[name]
	if !ok {
		return register.ModelProvider{}, register.ErrModelProviderNotFound
	}
	return provider, nil
}
func (f *fakeProviderApps) DeleteModelProvider(_ context.Context, name string) error {
	if _, ok := f.providers[name]; !ok {
		return register.ErrModelProviderNotFound
	}
	delete(f.providers, name)
	return nil
}
func (f *fakeProviderApps) CheckModelProvider(ctx context.Context, name string) (register.ModelProvider, error) {
	return f.GetModelProvider(ctx, name)
}

func TestModelProviderAdminAndPublicAPI(t *testing.T) {
	apps := newFakeProviderApps()
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{AdminToken: "admin-token"}, apps)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	body := []byte(`{"name":"deepseek","adapter":"openai_compatible","base_url":"https://api.deepseek.com","default_model":"deepseek-flash","secret_ref":"deepseek_api_key","default":true}`)
	resp := request(t, http.MethodPost, httpServer.URL+"/admin/model-providers", "admin-token", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert status = %d", resp.StatusCode)
	}
	var created register.ModelProvider
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode provider: %v", err)
	}
	resp.Body.Close()
	if created.Name != "deepseek" || created.SecretRef != "deepseek_api_key" {
		t.Fatalf("provider = %+v", created)
	}

	resp = request(t, http.MethodGet, httpServer.URL+"/admin/model-providers", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	var list struct {
		Providers []map[string]any `json:"providers"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Providers) != 1 {
		t.Fatalf("providers = %+v", list.Providers)
	}
	if list.Providers[0]["secret_ref"] != "deepseek_api_key" {
		t.Fatalf("admin view missing secret ref: %+v", list.Providers[0])
	}

	resp = request(t, http.MethodGet, httpServer.URL+"/api/v1/model-providers", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public list status = %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if bytes.Contains(raw, []byte("secret_ref")) || bytes.Contains(raw, []byte("deepseek_api_key")) {
		t.Fatalf("public provider view leaked secret metadata: %s", raw)
	}
}
