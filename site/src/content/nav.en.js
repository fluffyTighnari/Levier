/* English presentation metadata for the documentation.
   The Markdown files and manifest.json remain the source of truth for structure
   and body content. These titles and descriptions only label the shell. */
export const SECTION_LABELS = {
  "接入": "Getting started",
  "配置": "Configuration",
  "App 端接入与管理": "App",
  "Levier 端管理": "Levier",
  "Runtime 端接入与管理": "Runtime",
  "总览": "Reference"
};

export const DOC_META = {
  onboarding: {
    title: "Full setup",
    description: "The shortest complete path from install to your first run, with links into each management surface."
  },
  "pre-model": {
    title: "Model configuration",
    description: "Connect a model and verify it: URL, API key, model name, adapter and checks."
  },
  "pre-mcp": {
    title: "MCP configuration",
    description: "Register MCP servers and configure transports, secrets and allowed_tools."
  },
  "pre-profile": {
    title: "Profile and project configuration",
    description: "User profile, project .levier.json and where each setting lives."
  },
  "pre-env": {
    title: "Environment and secrets",
    description: "Environment precedence, App and model keys, and managed secrets."
  },
  "app-core": {
    title: "App core workflow",
    description: "Install, tools, init, dev, run and error handling in one loop."
  },
  "app-install": {
    title: "App SDK, tools and construction",
    description: "Install the App SDK, define tools and construct an App."
  },
  "app-dev": {
    title: "Initialize and run an App",
    description: "Initialize a project and serve the App with dev or Python."
  },
  "app-runs": {
    title: "Runs",
    description: "Submit runs from the CLI or SDK, then inspect state and events."
  },
  "app-mcp": {
    title: "MCP references and reconnects",
    description: "Declare MCP references and handle disconnects and reconnects."
  },
  "app-manage": {
    title: "App management",
    description: "Self-check, inspect and rotate App keys."
  },
  "levier-install": {
    title: "Install, connect and start",
    description: "Install the CLI, connect a profile and start the local stack."
  },
  "levier-status": {
    title: "Stack lifecycle",
    description: "Check status, read logs and stop the stack."
  },
  "levier-run": {
    title: "Runs",
    description: "Submit a run and inspect or cancel it."
  },
  "levier-runtime-list": {
    title: "Runtime list",
    description: "Instances, capacity and heartbeats."
  },
  "levier-doctor": {
    title: "Diagnostics and maintenance",
    description: "Run doctor / describe / selfcheck, and validate, migrate or maintain the control plane."
  },
  "levier-ops": {
    title: "Operations view",
    description: "ops runs, ops daemons and ops tasks."
  },
  "levier-remote": {
    title: "Remote control plane",
    description: "HTTPS, admin and runtime tokens, and network boundaries."
  },
  "runtime-core": {
    title: "Runtime core workflow",
    description: "runtime init, run(ctx), start and error handling."
  },
  "runtime-start": {
    title: "Set up a runtime",
    description: "Install the Runtime SDK, initialize a template and start the runtime."
  },
  "runtime-builtin": {
    title: "Built-in runtimes",
    description: "react1, echo1 and conformance."
  },
  "runtime-context": {
    title: "Context",
    description: "ctx fields, input, model, tools, MCP, timeouts and cancellation."
  },
  "runtime-toolbox": {
    title: "Toolbox",
    description: "Aggregate local and MCP tools, invoke them and close the toolbox."
  },
  "runtime-chat": {
    title: "chat and invoke",
    description: "ctx.chat, ctx.invoke and ctx.invoke_all."
  },
  "runtime-errors": {
    title: "Errors and checkpoints",
    description: "Handle tool and model errors, cancellation and checkpoint state."
  },
  "runtime-remote": {
    title: "Deploy and capacity",
    description: "Inspect instances and capacity, and deploy a remote runtime with tokens and TLS."
  },
  cli: {
    title: "CLI reference",
    description: "Main paths, observability, advanced commands, compatibility, --json and exit codes."
  },
  sdk: {
    title: "SDK reference",
    description: "Grouped by task: App, Runtime, tools, runs, config and install."
  },
  env: {
    title: "Environment variables",
    description: "User, project, CLI, SDK and deployment environment variables."
  },
  errors: {
    title: "Errors and troubleshooting",
    description: "Exit codes, common failures and what to check first."
  },
  glossary: {
    title: "Glossary",
    description: "App, Runtime, profile, managed secret and other terms."
  }
};

export function docMeta(item) {
  const meta = DOC_META[item.slug] || {};
  return {
    title: meta.title || item.title,
    description: meta.description || item.description,
    originalTitle: item.title
  };
}

export function sectionLabel(title) {
  return SECTION_LABELS[title] || title;
}

export function sectionLabelFor(title, locale = "en") {
  return locale === "en" ? sectionLabel(title) : title;
}

export function docPresentation(item, locale = "en") {
  if (locale === "en") {
    const meta = docMeta(item);
    return { title: meta.title, description: meta.description };
  }
  return { title: item.title, description: item.description };
}
