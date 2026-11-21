package main

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// cliProfile is the optional user-level configuration for the client CLI.
// It contains locators, not secrets.
type cliProfile struct {
	Server            string `yaml:"server,omitempty"`
	AdminToken        string `yaml:"admin_token,omitempty"`
	Credentials       string `yaml:"credentials,omitempty"`
	AppName           string `yaml:"app_name,omitempty"`
	MCPRefs           string `yaml:"mcp_refs,omitempty"`
	Runtime           string `yaml:"runtime,omitempty"`
	RuntimeToken      string `yaml:"runtime_token,omitempty"`
	MiddlewareAddress string `yaml:"middleware_address,omitempty"`
	RuntimeConfig     string `yaml:"runtime_config,omitempty"`
	MCPSocket         string `yaml:"mcp_socket,omitempty"`
	ControlConfig     string `yaml:"control_config,omitempty"`
	StackFile         string `yaml:"stack_file,omitempty"`
}

func profilePath() (string, error) {
	if home := strings.TrimSpace(os.Getenv("LEVIER_HOME")); home != "" {
		return filepath.Join(home, "config.yaml"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "levier", "config.yaml"), nil
}

func loadProfile() cliProfile {
	path, err := profilePath()
	if err != nil {
		return cliProfile{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cliProfile{}
	}
	var profile cliProfile
	if yaml.Unmarshal(data, &profile) != nil {
		return cliProfile{}
	}
	return profile
}

func saveProfile(profile cliProfile) (string, error) {
	path, err := profilePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(profile)
	if err != nil {
		return "", err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	return path, nil
}

func defaultServerValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_SERVER")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProjectConfig().Server); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProfile().Server); value != "" {
		return value
	}
	return "http://127.0.0.1:8080"
}

func defaultCredentialsValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_CREDENTIALS")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProjectConfig().Credentials); value != "" {
		return value
	}
	return strings.TrimSpace(loadProfile().Credentials)
}

func defaultRuntimeValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_RUNTIME_NAME")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProjectConfig().Runtime); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProfile().Runtime); value != "" {
		return value
	}
	return "react1"
}

func defaultAppNameValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_APP_NAME")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProjectConfig().AppName); value != "" {
		return value
	}
	return strings.TrimSpace(loadProfile().AppName)
}

func defaultMCPRefsValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_APP_MCP_REFS")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProjectConfig().MCPRefs); value != "" {
		return value
	}
	return strings.TrimSpace(loadProfile().MCPRefs)
}

func defaultRuntimeTokenValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_RUNTIME_TOKEN")); value != "" {
		return value
	}
	return strings.TrimSpace(loadProfile().RuntimeToken)
}

func defaultMiddlewareAddressValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_MIDDLEWARE_ADDRESS")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProfile().MiddlewareAddress); value != "" {
		return value
	}
	server := defaultServerValue()
	parsed, err := url.Parse(server)
	if err != nil || parsed.Hostname() == "" {
		return "127.0.0.1:9090"
	}
	return net.JoinHostPort(parsed.Hostname(), "9090")
}

func defaultAdminTokenValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_ADMIN_TOKEN")); value != "" {
		return value
	}
	profile := loadProfile()
	if value := strings.TrimSpace(profile.AdminToken); value != "" {
		return value
	}
	if strings.TrimSpace(profile.Server) == "" {
		return ""
	}
	parsed, err := url.Parse(profile.Server)
	if err == nil && isLoopbackHost(parsed.Hostname()) {
		// Matches the development stack shipped in this repository.
		return "admin-change-me"
	}
	return ""
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func controlConfigValue() string {
	if value := defaultControlConfigValue(); value != "" {
		return value
	}
	return "levier.example.yaml"
}

func defaultStackFileValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_STACK_FILE")); value != "" {
		return value
	}
	return strings.TrimSpace(loadProfile().StackFile)
}

func levierDataDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("LEVIER_HOME")); home != "" {
		return home, nil
	}
	switch runtime.GOOS {
	case "windows":
		if dir, err := os.UserCacheDir(); err == nil {
			return filepath.Join(dir, "levier"), nil
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "levier"), nil
		}
	default:
		if dir := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dir != "" {
			return filepath.Join(dir, "levier"), nil
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", "levier"), nil
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "levier"), nil
}

func installedStackFileValue() string {
	dir, err := levierDataDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "stack", "docker-compose.yml")
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func defaultMCPSocketValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_MCP_SOCKET")); value != "" {
		return value
	}
	return strings.TrimSpace(loadProfile().MCPSocket)
}

func defaultControlConfigValue() string {
	if value := strings.TrimSpace(os.Getenv("LEVIER_CONFIG")); value != "" {
		return value
	}
	if value := strings.TrimSpace(loadProfile().ControlConfig); value != "" {
		return value
	}
	return ""
}
