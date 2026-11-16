package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type appCredentials struct {
	AppID   string `json:"app_id"`
	AppName string `json:"app_name"`
	APIKey  string `json:"api_key"`
}

func readCredentials(path string) (appCredentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return appCredentials{}, err
	}
	var credentials appCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return appCredentials{}, fmt.Errorf("parse credentials: %w", err)
	}
	if credentials.AppID == "" || credentials.APIKey == "" {
		return appCredentials{}, fmt.Errorf("credentials missing app_id or api_key")
	}
	return credentials, nil
}

func writeCredentials(path string, credentials appCredentials, force bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("credentials path is required")
	}
	if credentials.AppID == "" || credentials.APIKey == "" {
		return "", fmt.Errorf("app_id and api_key are required")
	}

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
