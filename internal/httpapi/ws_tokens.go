// Middleware — internal/httpapi
//
// Short-lived App WebSocket tokens.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const defaultWSTokenTTL = 10 * time.Minute

type wsToken struct {
	appID   string
	expires time.Time
}

// WSTokenStore issues and validates short-lived WebSocket tokens.
type WSTokenStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]wsToken
	now    func() time.Time
}

// NewWSTokenStore returns a token store with the given TTL.
func NewWSTokenStore(ttl time.Duration) *WSTokenStore {
	if ttl <= 0 {
		ttl = defaultWSTokenTTL
	}
	return &WSTokenStore{
		ttl:    ttl,
		tokens: make(map[string]wsToken),
		now:    time.Now,
	}
}

// Issue creates a token for one App.
func (s *WSTokenStore) Issue(appID string) string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	now := s.now()
	s.mu.Lock()
	for existing, value := range s.tokens {
		if now.After(value.expires) {
			delete(s.tokens, existing)
		}
	}
	s.tokens[token] = wsToken{appID: appID, expires: now.Add(s.ttl)}
	s.mu.Unlock()
	return token
}

// TTL returns the configured token lifetime.
func (s *WSTokenStore) TTL() time.Duration {
	if s == nil {
		return defaultWSTokenTTL
	}
	return s.ttl
}

// Validate returns the App bound to a token.
func (s *WSTokenStore) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.tokens[token]
	if !ok {
		return "", false
	}
	if s.now().After(value.expires) {
		delete(s.tokens, token)
		return "", false
	}
	return value.appID, true
}
