package register

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMutableFileSecretStorePersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	store, err := LoadMutableFileSecretStore(path)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	if _, err := store.Resolve(context.Background(), "deepseek_api_key"); err == nil {
		t.Fatal("expected missing secret")
	}
	if err := store.PutSecret(context.Background(), "deepseek_api_key", "sk-test"); err != nil {
		t.Fatalf("put secret: %v", err)
	}
	if value, err := store.Resolve(context.Background(), "deepseek_api_key"); err != nil || value != "sk-test" {
		t.Fatalf("resolve = %q, %v", value, err)
	}

	reloaded, err := LoadMutableFileSecretStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	if value, err := reloaded.Resolve(context.Background(), "deepseek_api_key"); err != nil || value != "sk-test" {
		t.Fatalf("reloaded resolve = %q, %v", value, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("managed secret file permissions = %04o", info.Mode().Perm())
		}
	}
}

func TestChainResolverFinishesWithActionableError(t *testing.T) {
	resolver := ChainResolver{Resolvers: []SecretResolver{
		EnvSecretResolver{Prefix: "LEVIER_TEST_MISSING_"},
		FileSecretResolver{Values: map[string]string{}},
	}}
	if _, err := resolver.Resolve(context.Background(), "unknown"); err == nil {
		t.Fatal("expected missing secret error")
	}
}
