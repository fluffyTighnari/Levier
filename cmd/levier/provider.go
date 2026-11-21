package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

func runModel(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stdout, "usage:")
		fmt.Fprintln(stdout, "  levier model connect --url URL --key KEY --model MODEL [--name name]")
		fmt.Fprintln(stdout, "  levier model list")
		fmt.Fprintln(stdout, "  levier model check --name name")
		fmt.Fprintln(stdout, "  levier model remove --name name --yes")
		return 0
	}
	switch args[0] {
	case "connect":
		return runProviderConnect(args[1:], stdout, stderr)
	case "list":
		return runModelList(args[1:], stdout, stderr)
	case "check":
		return runProviderCheck(args[1:], stdout, stderr)
	case "remove", "delete":
		return runProviderRemove(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown model command: %s\n\n", args[0])
		fmt.Fprintln(stderr, "usage: levier model connect --url URL --key KEY --model MODEL")
		return 2
	}
}

func runModelList(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("model list")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	providers, err := client.listModelProviders(context.Background())
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		views := make([]map[string]any, 0, len(providers))
		for _, provider := range providers {
			views = append(views, map[string]any{
				"name":          provider.Name,
				"base_url":      provider.BaseURL,
				"default_model": provider.DefaultModel,
				"models":        provider.Models,
				"default":       provider.Default,
			})
		}
		return encodeJSON(stdout, map[string]any{"providers": views}, stderr)
	}
	if len(providers) == 0 {
		fmt.Fprintln(stdout, "no models connected")
		fmt.Fprintln(stdout, "next: levier model connect --url URL --key KEY --model MODEL")
		return 0
	}
	fmt.Fprintf(stdout, "%-18s %-20s %-8s %s\n", "name", "model", "default", "base_url")
	for _, provider := range providers {
		fmt.Fprintf(stdout, "%-18s %-20s %-8t %s\n",
			provider.Name, provider.DefaultModel, provider.Default, provider.BaseURL)
	}
	return 0
}

func runProvider(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printProviderUsage(stderr)
		return 2
	}
	switch args[0] {
	case "connect":
		return runProviderConnect(args[1:], stdout, stderr)
	case "list":
		return runProviderList(args[1:], stdout, stderr)
	case "show":
		return runProviderShow(args[1:], stdout, stderr)
	case "add", "update":
		return runProviderUpsert(args[0], args[1:], stdout, stderr)
	case "remove", "delete":
		return runProviderRemove(args[1:], stdout, stderr)
	case "check":
		return runProviderCheck(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printProviderUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown provider command: %s\n\n", args[0])
		printProviderUsage(stderr)
		return 2
	}
}

func printProviderUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier provider connect --url <base-url> --key <api-key> --model <model> [--name name]")
	fmt.Fprintln(out, "  levier provider list [--json]")
	fmt.Fprintln(out, "  levier provider show --name <name> [--json]")
	fmt.Fprintln(out, "  levier provider add --name <name> --adapter openai_compatible|anthropic --secret-ref <ref> [options]")
	fmt.Fprintln(out, "  levier provider update --name <name> ... [options]")
	fmt.Fprintln(out, "  levier provider remove --name <name> --yes")
	fmt.Fprintln(out, "  levier provider check --name <name>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Provider mutations require an Admin token.")
	fmt.Fprintln(out, "Secret values are supplied through LEVIER_SECRET_<REF> or LEVIER_SECRETS_FILE.")
}

func runProviderConnect(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("provider connect")
	baseURL := flags.String("url", "", "provider base URL, for example https://api.deepseek.com")
	apiKey := flags.String("key", "", "provider API key (or LEVIER_MODEL_API_KEY)")
	model := flags.String("model", "", "default model")
	name := flags.String("name", "", "provider name (inferred from the URL)")
	adapter := flags.String("adapter", "auto", "auto, openai_compatible, or anthropic")
	models := flags.String("models", "", "comma-separated allowed models")
	defaultProvider := flags.Bool("default", true, "make this the default provider")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "provider connect: unexpected argument: %s\n", flags.Arg(0))
		return 2
	}
	key := strings.TrimSpace(*apiKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("LEVIER_MODEL_API_KEY"))
	}
	if key == "" {
		key = strings.TrimSpace(os.Getenv("LEVIER_API_KEY"))
	}
	if key == "" {
		fmt.Fprintln(stderr, "API key is required: --key or LEVIER_MODEL_API_KEY")
		return 2
	}
	modelName := strings.TrimSpace(*model)
	if modelName == "" {
		fmt.Fprintln(stderr, "model is required: --model")
		return 2
	}
	urlValue := normalizeProviderURL(*baseURL)
	if urlValue == "" {
		fmt.Fprintln(stderr, "provider URL is required: --url")
		return 2
	}
	providerName := sanitizeProviderName(*name)
	if providerName == "" {
		providerName = inferProviderName(urlValue)
	}
	if providerName == "" {
		fmt.Fprintln(stderr, "could not infer provider name; pass --name")
		return 2
	}
	adapterValue := inferAdapter(*adapter, urlValue)
	secretRef := providerSecretRef(providerName)
	modelList := splitCSV(*models)
	if len(modelList) == 0 {
		modelList = []string{modelName}
	}
	if !containsString(modelList, modelName) {
		modelList = append([]string{modelName}, modelList...)
	}

	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	ctx := context.Background()
	provider, err := client.upsertModelProvider(ctx, map[string]any{
		"name":          providerName,
		"adapter":       adapterValue,
		"base_url":      urlValue,
		"default_model": modelName,
		"models":        modelList,
		"secret_ref":    secretRef,
		"enabled":       true,
		"default":       *defaultProvider,
	})
	if err != nil {
		return reportAPIError(err, stderr)
	}
	provider, err = client.putModelProviderSecret(ctx, providerName, key)
	if err != nil {
		fmt.Fprintf(stderr, "provider metadata was saved, but the key could not be stored: %v\n", err)
		fmt.Fprintln(stderr, "Upgrade the control plane, or set this key on the server and retry:")
		fmt.Fprintf(stderr, "  LEVIER_SECRET_%s=... (then restart the control plane)\n", strings.ToUpper(secretRef))
		return reportAPIError(err, stderr)
	}
	provider, err = client.checkModelProvider(ctx, providerName)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, provider, stderr)
	}
	fmt.Fprintf(stdout, "model connected: %s\n", provider.Name)
	fmt.Fprintf(stdout, "url:              %s\n", provider.BaseURL)
	fmt.Fprintf(stdout, "model:            %s\n", provider.DefaultModel)
	fmt.Fprintf(stdout, "default:          %t\n", provider.Default)
	fmt.Fprintln(stdout, "next:             levier init")
	return 0
}

func normalizeProviderURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.TrimRight(parsed.Path, "/")
	if strings.Contains(host, "anthropic") {
		if path == "" {
			path = "/v1/messages"
		} else if path == "/v1" {
			path += "/messages"
		}
	} else if path == "" && (host == "api.openai.com" || host == "openai.com") {
		path = "/v1"
	}
	parsed.Path = path
	return parsed.String()
}

func sanitizeProviderName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), ".-")
}

func inferProviderName(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "local"
	}
	parts := strings.Split(host, ".")
	for len(parts) > 0 && (parts[0] == "www" || parts[0] == "api" || parts[0] == "chat" || parts[0] == "llm") {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) >= 2 {
		return sanitizeProviderName(parts[len(parts)-2])
	}
	return sanitizeProviderName(parts[0])
}

func inferAdapter(value, baseURL string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "anthropic":
		return "anthropic"
	case "openai", "openai_compatible", "openai-compatible":
		return "openai_compatible"
	}
	if strings.Contains(strings.ToLower(baseURL), "anthropic") {
		return "anthropic"
	}
	return "openai_compatible"
}

func providerSecretRef(name string) string {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	return replacer.Replace(name) + "_api_key"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func providerFlagSet(name string) (*flag.FlagSet, *string, *string, *time.Duration, *bool) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	return flags, server, adminToken, timeout, asJSON
}

func providerAdminClient(server, token string, timeout time.Duration, stderr io.Writer) (*apiClient, bool) {
	if token == "" {
		token = defaultAdminTokenValue()
	}
	if token == "" {
		fmt.Fprintln(stderr, "admin token is required: --admin-token or LEVIER_ADMIN_TOKEN")
		return nil, false
	}
	if strings.TrimSpace(server) == "" {
		server = defaultServerValue()
	}
	return newAPIClient(resolveServer(server, nil), token, timeout), true
}

func runProviderList(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("provider list")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "provider list: unexpected argument: %s\n", flags.Arg(0))
		return 2
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	providers, err := client.listModelProviders(context.Background())
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, map[string]any{"providers": providers}, stderr)
	}
	if len(providers) == 0 {
		fmt.Fprintln(stdout, "no model providers")
		return 0
	}
	fmt.Fprintf(stdout, "%-18s %-20s %-12s %-8s %-8s %-10s %s\n",
		"name", "adapter", "default_model", "default", "enabled", "secret", "base_url")
	for _, provider := range providers {
		secret := "missing"
		if provider.SecretConfigured {
			secret = "configured"
		}
		fmt.Fprintf(stdout, "%-18s %-20s %-12s %-8t %-8t %-10s %s\n",
			provider.Name, provider.Adapter, provider.DefaultModel,
			provider.Default, provider.Enabled, secret, provider.BaseURL)
	}
	return 0
}

func runProviderShow(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("provider show")
	name := flags.String("name", "", "provider name")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *name == "" {
		fmt.Fprintln(stderr, "provider name is required: --name")
		return 2
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	provider, err := client.getModelProvider(context.Background(), *name)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, provider, stderr)
	}
	printProvider(stdout, provider)
	return 0
}

func runProviderUpsert(command string, args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("provider " + command)
	name := flags.String("name", "", "provider name")
	adapter := flags.String("adapter", "openai_compatible", "openai_compatible or anthropic")
	baseURL := flags.String("base-url", "", "provider base URL")
	defaultModel := flags.String("default-model", "", "default model")
	models := flags.String("models", "", "comma-separated allowed models")
	secretRef := flags.String("secret-ref", "", "secret reference name")
	enabled := flags.Bool("enabled", true, "enable this provider")
	defaultProvider := flags.Bool("default", false, "make this the default provider")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "provider %s: unexpected argument: %s\n", command, flags.Arg(0))
		return 2
	}
	if strings.TrimSpace(*name) == "" {
		fmt.Fprintln(stderr, "provider name is required: --name")
		return 2
	}
	adapterValue := strings.TrimSpace(*adapter)
	if adapterValue == "openai" {
		adapterValue = "openai_compatible"
	}
	payload := map[string]any{
		"name":          strings.TrimSpace(*name),
		"adapter":       adapterValue,
		"base_url":      strings.TrimSpace(*baseURL),
		"default_model": strings.TrimSpace(*defaultModel),
		"models":        splitCSV(*models),
		"secret_ref":    strings.TrimSpace(*secretRef),
		"enabled":       *enabled,
		"default":       *defaultProvider,
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	provider, err := client.upsertModelProvider(context.Background(), payload)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, provider, stderr)
	}
	fmt.Fprintf(stdout, "provider saved: %s\n", provider.Name)
	printProvider(stdout, provider)
	return 0
}

func runProviderRemove(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("provider remove")
	name := flags.String("name", "", "provider name")
	yes := flags.Bool("yes", false, "confirm removal")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *name == "" {
		fmt.Fprintln(stderr, "provider name is required: --name")
		return 2
	}
	if !*yes {
		fmt.Fprintln(stderr, "refusing to remove without --yes")
		return 5
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	if err := client.deleteModelProvider(context.Background(), *name); err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, map[string]string{"status": "deleted", "name": *name}, stderr)
	}
	fmt.Fprintf(stdout, "provider removed: %s\n", *name)
	return 0
}

func runProviderCheck(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := providerFlagSet("provider check")
	name := flags.String("name", "", "provider name")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *name == "" {
		fmt.Fprintln(stderr, "provider name is required: --name")
		return 2
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	provider, err := client.checkModelProvider(context.Background(), *name)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, provider, stderr)
	}
	fmt.Fprintf(stdout, "provider check: PASS %s\n", provider.Name)
	printProvider(stdout, provider)
	return 0
}

func printProvider(stdout io.Writer, provider modelProviderView) {
	fmt.Fprintf(stdout, "name              %s\n", provider.Name)
	fmt.Fprintf(stdout, "adapter           %s\n", provider.Adapter)
	fmt.Fprintf(stdout, "base_url          %s\n", provider.BaseURL)
	fmt.Fprintf(stdout, "default_model     %s\n", provider.DefaultModel)
	fmt.Fprintf(stdout, "models            %s\n", strings.Join(provider.Models, ", "))
	fmt.Fprintf(stdout, "secret_ref        %s\n", provider.SecretRef)
	fmt.Fprintf(stdout, "secret_configured %t\n", provider.SecretConfigured)
	fmt.Fprintf(stdout, "enabled           %t\n", provider.Enabled)
	fmt.Fprintf(stdout, "default           %t\n", provider.Default)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}
