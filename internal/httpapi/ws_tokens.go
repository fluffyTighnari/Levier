// Levier — internal/httpapi
//
// Short-lived App WebSocket tokens.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const defaultWSTokenTTL = 10 * time.Minute

type wsToken struct {
	appID   string
	expires time.Time
}

// WSTokenStore issues and validates short-lived WebSocket tokens.
//
// With a shared secret the token is stateless and can be validated by any
// replica. Without a secret it falls back to the original process-local store
// for local development and compatibility.
type WSTokenStore struct {
	mu     sync.Mutex
	ttl    time.Duration
	tokens map[string]wsToken
	secret []byte
	now    func() time.Time
}

// NewWSTokenStore returns a process-local token store with the given TTL.
func NewWSTokenStore(ttl time.Duration) *WSTokenStore {
	return NewWSTokenStoreWithSecret(ttl, "")
}

// NewWSTokenStoreWithSecret returns a stateless token store when secret is set.
func NewWSTokenStoreWithSecret(ttl time.Duration, secret string) *WSTokenStore {
	if ttl <= 0 {
		ttl = defaultWSTokenTTL
	}
	return &WSTokenStore{
		ttl:    ttl,
		tokens: make(map[string]wsToken),
		secret: []byte(secret),
		now:    time.Now,
	}
}

// Issue creates a token for one App.
func (s *WSTokenStore) Issue(appID string) string {
	if len(s.secret) > 0 {
		return s.issueStateless(appID)
	}
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

func (s *WSTokenStore) issueStateless(appID string) string {
	payload, err := json.Marshal(wsTokenPayload{
		AppID:   appID,
		Expires: s.now().Add(s.ttl).Unix(),
	})
	if err != nil {
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := s.sign(encoded)
	return "v1." + encoded + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func (s *WSTokenStore) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("v1." + payload))
	return mac.Sum(nil)
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
	if len(s.secret) > 0 {
		return s.validateStateless(token)
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

func (s *WSTokenStore) validateStateless(token string) (string, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] == "" || parts[2] == "" {
		return "", false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, s.sign(parts[1])) {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var value wsTokenPayload
	if err := json.Unmarshal(payload, &value); err != nil || value.AppID == "" {
		return "", false
	}
	if s.now().Unix() > value.Expires {
		return "", false
	}
	return value.AppID, true
}

type wsTokenPayload struct {
	AppID   string `json:"app_id"`
	Expires int64  `json:"exp"`
}
