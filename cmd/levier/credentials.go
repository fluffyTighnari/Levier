package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const credentialVersion = 1

type appCredentials struct {
	CredentialVersion int    `json:"credential_version,omitempty"`
	AppID             string `json:"app_id"`
	AppName           string `json:"app_name"`
	APIKey            string `json:"api_key"`
	Server            string `json:"server,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

func readCredentials(path string) (appCredentials, error) {
	info, err := os.Stat(path)
	if err != nil {
		return appCredentials{}, err
	}
	if info.IsDir() {
		return appCredentials{}, fmt.Errorf("credentials path is a directory: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return appCredentials{}, fmt.Errorf(
			"credentials file permissions are %04o; want 0600: %s",
			info.Mode().Perm(), path,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return appCredentials{}, err
	}
	var credentials appCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return appCredentials{}, fmt.Errorf("parse credentials: %w", err)
	}
	if credentials.CredentialVersion == 0 {
		// Files written before credential_version existed are still readable.
		credentials.CredentialVersion = credentialVersion
	}
	if credentials.CredentialVersion != credentialVersion {
		return appCredentials{}, fmt.Errorf(
			"unsupported credentials version %d (client supports %d)",
			credentials.CredentialVersion, credentialVersion,
		)
	}
	if credentials.AppID == "" || credentials.APIKey == "" {
		return appCredentials{}, fmt.Errorf("credentials missing app_id or api_key")
	}
	credentials.Server = strings.TrimRight(strings.TrimSpace(credentials.Server), "/")
	return credentials, nil
}

func writeCredentials(path string, credentials appCredentials, force bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("credentials path is required")
	}
	if credentials.AppID == "" || credentials.APIKey == "" {
		return "", fmt.Errorf("app_id and api_key are required")
	}

	credentials.CredentialVersion = credentialVersion
	credentials.Server = strings.TrimRight(strings.TrimSpace(credentials.Server), "/")
	now := time.Now().UTC().Format(time.RFC3339)
	if credentials.CreatedAt == "" {
		credentials.CreatedAt = now
	}
	credentials.UpdatedAt = now

	backup := ""
	if _, err := os.Stat(path); err == nil {
		if !force {
			return "", fmt.Errorf("credentials already exist: %s (use --force to overwrite)", path)
		}
		backup = path + ".bak"
		if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove old backup: %w", err)
		}
		if err := os.Rename(path, backup); err != nil {
			return "", fmt.Errorf("backup credentials: %w", err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(backup, 0o600); err != nil {
				return "", fmt.Errorf("protect credentials backup: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check credentials: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create credentials directory: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary credentials: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return "", fmt.Errorf("set credentials mode: %w", err)
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(credentials); err != nil {
		temp.Close()
		return "", fmt.Errorf("write credentials: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return "", fmt.Errorf("sync credentials: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close credentials: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return "", fmt.Errorf("install credentials: %w", err)
	}
	return backup, nil
}

func preflightCredentialsPath(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".credentials-preflight-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	if closeErr := temp.Close(); closeErr != nil {
		_ = os.Remove(name)
		return closeErr
	}
	return os.Remove(name)
}

func credentialFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:6])
}

func resolveServer(flag string, credentials *appCredentials) string {
	if value := strings.TrimSpace(flag); value != "" {
		return serverBase(value)
	}
	if value := strings.TrimSpace(os.Getenv("LEVIER_SERVER")); value != "" {
		return serverBase(value)
	}
	if credentials != nil && strings.TrimSpace(credentials.Server) != "" {
		return serverBase(credentials.Server)
	}
	return "http://127.0.0.1:8080"
}

func defaultCredentialsPath(name string) (string, error) {
	if value := os.Getenv("LEVIER_CREDENTIALS"); value != "" {
		return value, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	filename := safeAppName(name) + ".json"
	return filepath.Join(dir, "levier", filename), nil
}

func safeAppName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "app"
	}
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	value := strings.Trim(builder.String(), ".-")
	if value == "" {
		return "app"
	}
	return value
}
