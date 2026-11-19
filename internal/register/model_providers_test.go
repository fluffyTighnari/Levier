package register

import (
	"context"
	"errors"
	"testing"
)

func TestModelProviderCatalogResolvesSecrets(t *testing.T) {
	registry := NewWithSecrets(testPool(t), FileSecretResolver{Values: map[string]string{
		"deepseek_api_key": "sk-test",
	}})
	ctx := context.Background()

	provider, err := registry.UpsertModelProvider(ctx, ModelProviderInput{
		Name:         "deepseek",
		Adapter:      "openai_compatible",
		BaseURL:      "https://api.deepseek.com",
		DefaultModel: "deepseek-flash",
		Models:       []string{"deepseek-flash", "deepseek-reasoner"},
		SecretRef:    "deepseek_api_key",
		Enabled:      true,
		Default:      true,
	})
	if err != nil {
		t.Fatalf("UpsertModelProvider() error = %v", err)
	}
	if provider.Name != "deepseek" || !provider.SecretConfigured || !provider.Default {
		t.Fatalf("provider = %+v", provider)
	}

	profile, err := registry.ResolveDefaultModelProfile(ctx, nil)
	if err != nil {
		t.Fatalf("ResolveDefaultModelProfile() error = %v", err)
	}
	if profile.Provider != "openai" || profile.Model != "deepseek-flash" || profile.APIKey != "sk-test" {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.Adapter != "openai_compatible" || profile.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("profile = %+v", profile)
	}

	profile, err = registry.ResolveModelProfile(ctx, "deepseek", "deepseek-reasoner", map[string]string{"temperature": "0.2"})
	if err != nil {
		t.Fatalf("ResolveModelProfile() error = %v", err)
	}
	if profile.Model != "deepseek-reasoner" || profile.Params["temperature"] != "0.2" {
		t.Fatalf("profile = %+v", profile)
	}

	if _, err := registry.ResolveModelProfile(ctx, "deepseek", "not-allowed", nil); err == nil {
		t.Fatal("disallowed model was accepted")
	}

	missing := New(testPool(t))
	if _, err := missing.UpsertModelProvider(ctx, ModelProviderInput{
		Name: "openai", Adapter: "openai_compatible", DefaultModel: "gpt-test",
		SecretRef: "missing_key", Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertModelProvider(missing) error = %v", err)
	}
	if _, err := missing.ResolveModelProfile(ctx, "openai", "", nil); err == nil {
		t.Fatal("missing secret was accepted")
	}

	if err := registry.DeleteModelProvider(ctx, "deepseek"); err != nil {
		t.Fatalf("DeleteModelProvider() error = %v", err)
	}
	if _, err := registry.ResolveModelProfile(ctx, "deepseek", "", nil); !errors.Is(err, ErrModelProviderNotFound) {
		t.Fatalf("resolved deleted provider: %v", err)
	}
}
