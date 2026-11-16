// Middleware — register
//
// Split from materials.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SecretResolver resolves ${secret:name} placeholders.
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
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
