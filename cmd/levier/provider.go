package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func runProvider(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printProviderUsage(stderr)
		return 2
	}
	switch args[0] {
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
		token = os.Getenv("LEVIER_ADMIN_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(stderr, "admin token is required: --admin-token or LEVIER_ADMIN_TOKEN")
		return nil, false
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
