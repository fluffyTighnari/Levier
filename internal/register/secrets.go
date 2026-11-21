// Levier — register
//
// Split from materials.go (P8R.3).
// Copyright 2026 The Levier Authors.
package register

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SecretResolver resolves ${secret:name} placeholders.
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// SecretWriter stores a secret value for a reference.
type SecretWriter interface {
	PutSecret(ctx context.Context, ref, value string) error
}

// ChainResolver tries each resolver in order and returns the first hit.
type ChainResolver struct {
	Resolvers []SecretResolver
}

// Resolve implements SecretResolver.
func (c ChainResolver) Resolve(ctx context.Context, ref string) (string, error) {
	var lastErr error
	for _, resolver := range c.Resolvers {
		if resolver == nil {
			continue
		}
		value, err := resolver.Resolve(ctx, ref)
		if err == nil {
			return value, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("secret %q is not configured", ref)
}

// MutableFileSecretStore is a small 0600 JSON secret store that the control
// plane can update through the Admin API. It makes `levier model connect`
// possible without asking the user to wire environment variables.
type MutableFileSecretStore struct {
	mu     sync.RWMutex
	path   string
	values map[string]string
}

// LoadMutableFileSecretStore loads an existing store or starts an empty one.
func LoadMutableFileSecretStore(path string) (*MutableFileSecretStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("managed secret file path is required")
	}
	store := &MutableFileSecretStore{path: path, values: map[string]string{}}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("managed secret file must be 0600: %s", path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read managed secret file: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return store, nil
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse managed secret file: %w", err)
	}
	store.values = values
	return store, nil
}

// Resolve implements SecretResolver.
func (s *MutableFileSecretStore) Resolve(_ context.Context, ref string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[ref]
	if !ok || value == "" {
		return "", fmt.Errorf("secret %q is not configured", ref)
	}
	return value, nil
}

// PutSecret implements SecretWriter.
func (s *MutableFileSecretStore) PutSecret(_ context.Context, ref, value string) error {
	ref = strings.TrimSpace(ref)
	value = strings.TrimSpace(value)
	if ref == "" || value == "" {
		return fmt.Errorf("secret reference and value are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]string, len(s.values)+1)
	for key, item := range s.values {
		next[key] = item
	}
	next[ref] = value
	if err := writeSecretMap(s.path, next); err != nil {
		return err
	}
	s.values = next
	return nil
}

func writeSecretMap(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secret directory: %w", err)
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("encode secret file: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".secrets-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary secret file: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("protect temporary secret file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write secret file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync secret file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close secret file: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("install secret file: %w", err)
	}
	return nil
}

// EnvSecretResolver reads LEVIER_SECRET_<REF> environment variables, with
// MIDDLEWARE_SECRET_<REF> as a legacy fallback.
type EnvSecretResolver struct {
	Prefix string
}

// FileSecretResolver reads secrets from an in-memory JSON map.
type FileSecretResolver struct {
	Values map[string]string
}

// LoadFileSecretResolver reads a JSON object of secret name -> value.
func LoadFileSecretResolver(path string) (*FileSecretResolver, error) {
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("read secret file: %w", err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("secret file must be 0600: %s", path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse secret file: %w", err)
	}
	return &FileSecretResolver{Values: values}, nil
}

// NewWithSecrets returns a registry with a custom secret resolver.
func NewWithSecrets(pool *pgxpool.Pool, secrets SecretResolver) *Registry {
	if secrets == nil {
		secrets = EnvSecretResolver{}
	}
	return &Registry{pool: pool, secrets: secrets}
}

// NewWithSecretsAndWriter returns a registry that can also persist provider keys.
func NewWithSecretsAndWriter(pool *pgxpool.Pool, secrets SecretResolver, writer SecretWriter) *Registry {
	registry := NewWithSecrets(pool, secrets)
	registry.secretWriter = writer
	return registry
}

func (r *Registry) resolveString(ctx context.Context, value string) (string, error) {
	for {
		start := strings.Index(value, "${secret:")
		if start < 0 {
			return value, nil
		}
		end := strings.Index(value[start:], "}")
		if end < 0 {
			return "", fmt.Errorf("register: malformed secret placeholder")
		}
		ref := value[start+len("${secret:") : start+end]
		secret, err := r.secretResolver().Resolve(ctx, ref)
		if err != nil {
			return "", err
		}
		value = value[:start] + secret + value[start+end+1:]
	}
}

func (r *Registry) resolveMap(ctx context.Context, values map[string]string) (map[string]string, error) {
	if values == nil {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		resolved, err := r.resolveString(ctx, value)
		if err != nil {
			return nil, err
		}
		result[key] = resolved
	}
	return result, nil
}

func (r *Registry) secretResolver() SecretResolver {
	if r.secrets == nil {
		return EnvSecretResolver{}
	}
	return r.secrets
}
