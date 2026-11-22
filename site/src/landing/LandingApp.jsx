import { useRef, useState } from "react";
import {
  Box,
  Button,
  ButtonBase,
  Chip,
  Link,
  Stack,
  Tab,
  Tabs,
  Typography,
  useMediaQuery
} from "@mui/material";
import { useTheme } from "@mui/material/styles";
import {
  ArrowRight,
  ArrowUpRight
} from "@phosphor-icons/react";
import { CodeSurface } from "../components/CodeSurface";
import { CopyButton } from "../components/CopyButton";
import { LandingHeader } from "../components/LandingHeader";
import { SectionHeading } from "../components/SectionHeading";
import { SiteFooter } from "../components/SiteFooter";
import { SkipLink } from "../components/SkipLink";
import { Terminal } from "../components/Terminal";
import {
  APP_SAMPLE,
  CLI_STEPS,
  CONTRACT_STEPS,
  DOC_MAP,
  FEATURE_CODE,
  HERO_CODE,
  INSTALL_CMD,
  NEXT,
  RUNTIME_SAMPLE,
  TODAY
} from "../content/landing";
import { useActiveSection } from "../lib/useActiveSection";

const SECTION_IDS = ["runtime", "surface", "integration", "scope", "roadmap", "documentation"];

const INSIDE_RUN = ["Model calls", "Tool order", "Loop count", "Stop conditions"];
const AROUND_RUN = ["Scheduling and leases", "Run state and events", "Cancellation and timeouts", "App / MCP routing"];

const SCOPE = [
  {
    title: "Levier control plane",
    items: ["Scheduling and leases", "Run state and events", "Model and MCP catalog", "Local stack lifecycle", "Logs and observability"]
  },
  {
    title: "Runtime authors",
    items: ["run(ctx)", "Model calls", "Tool order", "Loops and stop conditions", "Custom state"]
  },
  {
    title: "App authors",
    items: ["Business state and data", "@tool", "App.run()", "Tool implementations", "Event callbacks"]
  }
];

const HANDLED_BY_SDK = ["runtime.yaml", "daemon id", "gRPC", "secret_ref", "hand-written schema", "credential paths"];

const ENDPOINTS = [
  {
    kicker: "Runtime · you design",
    title: "One run(ctx)",
    description: "You write the model calls, tool flow and loop. The SDK handles the connection, events and cancellation.",
    filename: "runtime.py",
    code: RUNTIME_SAMPLE,
    commands: ["levier runtime init", "levier runtime start"],
    href: "./docs/index.html#/runtime-core",
    linkLabel: "Runtime docs"
  },
  {
    kicker: "App · you implement",
    title: "One App.run()",
    description: "Your @tool functions hold business state. App.run() keeps the app connected and returns tool results to the runtime.",
    filename: "app.py",
    code: APP_SAMPLE,
    commands: ["levier init", "levier dev"],
    href: "./docs/index.html#/app-core",
    linkLabel: "App docs"
  }
];

function PageContainer({ children, sx }) {
  return (
    <Box sx={[{ width: "100%", maxWidth: 1200, mx: "auto", px: { xs: 2.5, md: 4 } }, ...(Array.isArray(sx) ? sx : [sx])]}>
      {children}
    </Box>
  );
}

function MonoMeta({ items, sx }) {
  return (
    <Box sx={[{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 0.75 }, ...(Array.isArray(sx) ? sx : [sx])]}>
      {items.map((item, index) => (
        <Box key={item} component="span" sx={{ display: "inline-flex", alignItems: "center", gap: 0.75 }}>
          <Typography component="span" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.71875rem", color: "text.secondary" }}>
            {item}
          </Typography>
          {index < items.length - 1 ? <Box component="span" aria-hidden="true" sx={{ color: "text.disabled" }}>·</Box> : null}
        </Box>
      ))}
    </Box>
  );
}

function Hero() {
  return (
    <Box component="section" sx={{ pt: { xs: 6, md: 10 }, pb: { xs: 6, md: 9 } }}>
      <PageContainer>
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "minmax(0, 1fr)", lg: "minmax(0, 1.04fr) minmax(0, 0.96fr)" },
            alignItems: "center",
            gap: { xs: 5, lg: 8 }
          }}
        >
          <Box>
            <Box sx={{ display: "inline-flex", alignItems: "center", gap: 1 }}>
              <Box sx={{ width: 6, height: 6, bgcolor: "primary.main" }} />
              <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.71875rem", letterSpacing: "0.03em", color: "primary.main" }}>
                Personal runtime · One function
              </Typography>
            </Box>

            <Typography component="h1" variant="h1" sx={{ mt: 2.25, textWrap: "balance" }}>
              Your runtime.<br />
              One <Box component="em" sx={{ fontStyle: "italic", fontWeight: 400, color: "primary.main" }}>run</Box> function.<br />
              That’s all you need.
            </Typography>

            <Typography sx={{ maxWidth: "46ch", mt: 2.5, fontSize: { xs: "1rem", md: "1.0625rem" }, lineHeight: 1.78, color: "text.secondary" }}>
              No base class, no template, no daemon config. Write one async function; you decide the model calls,
              tools, loop and stop condition. Levier handles scheduling, state and events around it.
            </Typography>

            <Stack direction="row" spacing={2} sx={{ mt: 3.5, flexWrap: "wrap", alignItems: "center", gap: 1.5 }}>
              <Button component="a" href="#runtime" variant="contained" size="large" endIcon={<ArrowRight size={17} />}>
                See a run function
              </Button>
              <Button component="a" href="#integration" variant="text" size="large" sx={{ px: 0.5, borderBottom: 1, borderColor: "text.disabled", borderRadius: 0, "&:hover": { borderColor: "primary.main" } }}>
                Get started in 5 minutes
              </Button>
            </Stack>

            <MonoMeta items={["run(ctx)", "App.run", "CLI control plane"]} sx={{ mt: 3.25 }} />
          </Box>

          <Box>
            <CodeSurface
              filename="runtime.py"
              tag="your loop"
              code={HERO_CODE}
              footer="$ levier runtime start"
              status="1 function"
            />
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1, mt: 1.5 }}>
              <Box sx={{ width: 4, height: 4, bgcolor: "primary.main", flex: "none", mt: "0.55em" }} />
              <Typography sx={{ fontSize: "0.71875rem", lineHeight: 1.6, color: "text.secondary" }}>
                One function is the contract. Scheduling, state and events stay outside it.
              </Typography>
            </Box>
          </Box>
        </Box>
      </PageContainer>
    </Box>
  );
}

function QuickInstall() {
  return (
    <Box sx={{ borderTop: 1, borderColor: "divider", bgcolor: "background.paper" }}>
      <PageContainer>
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "stretch", sm: "center" },
            gap: { xs: 0.75, sm: 2 },
            py: 1.5
          }}
        >
          <Typography
            component="span"
            sx={{ flex: "none", fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "primary.main" }}
          >
            Linux / macOS
          </Typography>
          <Box
            component="code"
            tabIndex={0}
            aria-label="Linux and macOS install command"
            sx={{
              minWidth: 0,
              flex: 1,
              py: 0.25,
              overflowX: "auto",
              whiteSpace: "nowrap",
              fontFamily: (theme) => theme.fonts.mono,
              fontSize: { xs: "0.6875rem", sm: "0.75rem" },
              color: "text.primary"
            }}
          >
            {INSTALL_CMD}
          </Box>
          <Box sx={{ flex: "none", display: "flex", justifyContent: { xs: "flex-start", sm: "flex-end" } }}>
            <CopyButton text={INSTALL_CMD} />
          </Box>
        </Box>
        <Typography sx={{ pb: 1.5, fontSize: "0.65625rem", lineHeight: 1.6, color: "text.secondary" }}>
          Windows: use{" "}
          <Link href="./docs/index.html#/levier-install" underline="always" sx={{ color: "text.primary", textDecorationColor: "divider" }}>
            install.ps1
          </Link>{" "}
          in the install guide.
        </Typography>
      </PageContainer>
    </Box>
  );
}

function ListBlock({ title, items }) {
  return (
    <Box>
      <Typography sx={{ fontSize: "0.75rem", fontWeight: 600, color: "primary.main" }}>{title}</Typography>
      <Stack sx={{ mt: 1, listStyle: "none" }} component="ul" spacing={0}>
        {items.map((item) => (
          <Box
            key={item}
            component="li"
            sx={{
              display: "flex",
              alignItems: "baseline",
              gap: 1,
              py: 0.9,
              borderBottom: 1,
              borderColor: "divider",
              fontSize: "0.84375rem",
              lineHeight: 1.55,
              color: "text.secondary"
            }}
          >
            <Box component="span" sx={{ width: 4, height: 4, bgcolor: "primary.main", flex: "none" }} />
            {item}
          </Box>
        ))}
      </Stack>
    </Box>
  );
}

function RuntimeSection() {
  return (
    <Box component="section" id="runtime" sx={{ py: { xs: 6, md: 10 }, borderTop: 1, borderColor: "divider", scrollMarginTop: "84px" }}>
      <PageContainer>
        <SectionHeading index="01" title="The runtime is one function" note="Not a template" />
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "minmax(0, 1fr)", md: "minmax(0, 0.92fr) minmax(0, 1.08fr)" },
            gap: { xs: 4, md: 7 },
            alignItems: "start"
          }}
        >
          <Box>
            <Typography component="h3" variant="h3" sx={{ fontSize: { xs: "2rem", md: "2.4rem" } }}>
              Your loop,<br />
              inside one <Box component="em" sx={{ fontStyle: "italic", color: "primary.main", fontWeight: 400 }}>function</Box>.
            </Typography>
            <Typography sx={{ maxWidth: "45ch", mt: 2, fontSize: "0.9375rem", lineHeight: 1.8, color: "text.secondary" }}>
              A runtime is one async function: <Box component="code" sx={{ px: 0.5, py: 0.15, borderRadius: 0.5, bgcolor: "action.hover", fontFamily: (theme) => theme.fonts.mono, fontSize: "0.8em", color: "text.primary" }}>run(ctx)</Box>.
              You decide how it calls the model, uses tools, loops and stops.
            </Typography>
            <Typography sx={{ maxWidth: "45ch", mt: 1.5, fontSize: "0.9375rem", lineHeight: 1.8, color: "text.secondary" }}>
              No base class, no template, no daemon config. <Box component="code" sx={{ px: 0.5, py: 0.15, borderRadius: 0.5, bgcolor: "action.hover", fontFamily: (theme) => theme.fonts.mono, fontSize: "0.8em", color: "text.primary" }}>ctx</Box> already
              carries this run’s model, tools, MCP bindings, timeout and cancellation state.
            </Typography>

            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", sm: "1fr 1fr" }, gap: 3.5, mt: 3.5, pt: 2.25, borderTop: 1, borderColor: "text.primary" }}>
              <ListBlock title="Inside run(ctx)" items={INSIDE_RUN} />
              <ListBlock title="Around run(ctx)" items={AROUND_RUN} />
            </Box>
          </Box>

          <Box>
            <CodeSurface filename="runtime.py" tag="you write this file" code={FEATURE_CODE} footer="$ levier runtime start" />
            <Typography sx={{ mt: 1.25, fontSize: "0.71875rem", lineHeight: 1.6, color: "text.secondary" }}>
              No runtime.yaml, daemon ID or gRPC.
            </Typography>
          </Box>
        </Box>
      </PageContainer>
    </Box>
  );
}

function EndpointPanel({ endpoint }) {
  return (
    <Box sx={{ minWidth: 0, display: "flex", flexDirection: "column", height: "100%" }}>
      <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "primary.main" }}>{endpoint.kicker}</Typography>
      <Typography component="h3" variant="h4" sx={{ mt: 1, fontSize: "1.625rem", fontFamily: (theme) => theme.fonts.display, fontWeight: 500 }}>
        {endpoint.title}
      </Typography>
      <Typography sx={{ mt: 0.75, fontSize: "0.84375rem", lineHeight: 1.7, color: "text.secondary" }}>{endpoint.description}</Typography>
      <Box sx={{ mt: 2.25, flexGrow: 1, display: "flex" }}>
        <CodeSurface filename={endpoint.filename} code={endpoint.code} dense fill />
      </Box>
      <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1, mt: 2 }}>
        {endpoint.commands.map((command) => (
          <Chip key={command} label={command} variant="outlined" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", height: 24 }} />
        ))}
        <Button component="a" href={endpoint.href} variant="text" size="small" endIcon={<ArrowRight size={14} />} sx={{ ml: "auto", px: 0.5 }}>
          {endpoint.linkLabel}
        </Button>
      </Box>
    </Box>
  );
}

function ContractStep({ label, title, body, first }) {
  return (
    <Box
      sx={{
        minWidth: 0,
        pt: { xs: first ? 0 : 2, md: 0 },
        borderTop: { xs: first ? 0 : 1, md: 0 },
        borderColor: "divider"
      }}
    >
      <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.59375rem", letterSpacing: "0.07em", textTransform: "uppercase", color: "primary.main" }}>
        {label}
      </Typography>
      <Typography sx={{ mt: 0.5, fontFamily: (theme) => theme.fonts.display, fontSize: "1.125rem", fontWeight: 500, color: "text.primary" }}>
        {title}
      </Typography>
      <Typography sx={{ mt: 0.5, fontSize: "0.75rem", lineHeight: 1.65, color: "text.secondary" }}>
        {body}
      </Typography>
    </Box>
  );
}

function ContractStrip() {
  const items = [];
  CONTRACT_STEPS.forEach((step, index) => {
    if (index > 0) {
      items.push(
        <Box key={`arrow-${step.label}`} sx={{ display: { xs: "none", md: "grid" }, placeItems: "center", color: "text.disabled" }}>
          <ArrowRight size={18} />
        </Box>
      );
    }
    items.push(<ContractStep key={step.label} {...step} first={index === 0} />);
  });

  return (
    <Box sx={{ mt: 4, pt: 3, borderTop: 1, borderColor: "text.primary" }}>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "1fr auto 1fr auto 1fr" },
          gap: { xs: 2.5, md: 3 },
          alignItems: "center"
        }}
      >
        {items}
      </Box>
      <Typography sx={{ maxWidth: "70ch", mt: 3, fontSize: "0.71875rem", lineHeight: 1.7, color: "text.secondary" }}>
        The runtime and the app never import each other. They meet through ctx and the tool contract.
      </Typography>
    </Box>
  );
}

function EndpointsSection() {
  const theme = useTheme();
  const desktop = useMediaQuery(theme.breakpoints.up("md"));
  const [value, setValue] = useState(0);

  return (
    <Box component="section" id="surface" sx={{ py: { xs: 6, md: 10 }, borderTop: 1, borderColor: "divider", scrollMarginTop: "84px" }}>
      <PageContainer>
        <SectionHeading index="02" title="Two entry points. One contract." note="App.run() + run(ctx)" />
        {desktop ? (
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: { md: 6 }, alignItems: "stretch" }}>
            <EndpointPanel endpoint={ENDPOINTS[0]} />
            <EndpointPanel endpoint={ENDPOINTS[1]} />
          </Box>
        ) : (
          <Box>
            <Tabs
              value={value}
              onChange={(event, next) => setValue(next)}
              variant="fullWidth"
              aria-label="Choose an entry point"
              sx={{ mb: 3, border: 1, borderColor: "divider", borderRadius: 1, minHeight: 42, bgcolor: "background.paper" }}
            >
              <Tab label="Runtime" id="endpoint-tab-runtime" aria-controls="endpoint-panel-runtime" />
              <Tab label="App" id="endpoint-tab-app" aria-controls="endpoint-panel-app" />
            </Tabs>
            {ENDPOINTS.map((endpoint, index) => {
              const key = index === 0 ? "runtime" : "app";
              return (
                <Box
                  key={key}
                  role="tabpanel"
                  id={`endpoint-panel-${key}`}
                  aria-labelledby={`endpoint-tab-${key}`}
                  tabIndex={0}
                  hidden={value !== index}
                  sx={{ "&[hidden]": { display: "none" } }}
                >
                  <EndpointPanel endpoint={endpoint} />
                </Box>
              );
            })}
          </Box>
        )}
        <ContractStrip />

        <Typography sx={{ maxWidth: "74ch", mt: 3.5, pt: 2, borderTop: 1, borderColor: "divider", fontSize: "0.84375rem", lineHeight: 1.75, color: "text.secondary" }}>
          The runtime entry point is one function. The app entry point is{" "}
          <Box component="code" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.85em", color: "text.primary" }}>App.run()</Box>.
          Neither side needs credential paths, YAML or daemon configuration.
        </Typography>
      </PageContainer>
    </Box>
  );
}

function CliSection() {
  const theme = useTheme();
  const desktop = useMediaQuery(theme.breakpoints.up("md"));
  const [step, setStep] = useState(0);
  const [animate, setAnimate] = useState(false);
  const seen = useRef(new Set([0]));
  const active = CLI_STEPS[step];

  const changeStep = (next) => {
    if (next === step) return;
    const firstVisit = !seen.current.has(next);
    seen.current.add(next);
    setAnimate(firstVisit);
    setStep(next);
  };

  const tabLabel = (item) => (
    <Box sx={{ display: "grid", gridTemplateColumns: "28px minmax(0,1fr) 16px", alignItems: "center", gap: 1, width: "100%", textAlign: "left" }}>
      <Typography component="span" sx={{ alignSelf: "start", pt: 0.25, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: step === Number(item.id) - 1 ? "primary.main" : "text.secondary" }}>
        {item.number}
      </Typography>
      <Box sx={{ minWidth: 0 }}>
        <Typography sx={{ fontSize: "0.84375rem", fontWeight: 600, color: "text.primary", lineHeight: 1.4 }}>{item.title}</Typography>
        <Typography sx={{ mt: 0.25, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.625rem", color: "text.secondary", lineHeight: 1.4 }}>{item.hint}</Typography>
      </Box>
      <ArrowRight size={14} style={{ color: "currentColor", opacity: 0.75 }} />
    </Box>
  );

  return (
    <Box component="section" id="integration" sx={{ py: { xs: 6, md: 10 }, borderTop: 1, borderColor: "divider", bgcolor: "action.hover", scrollMarginTop: "84px" }}>
      <PageContainer>
        <SectionHeading index="03" title="The CLI is the control plane" note="Local scheduling" />
        <Typography sx={{ maxWidth: "62ch", mt: -2, mb: 4, fontSize: "0.96875rem", lineHeight: 1.75, color: "text.secondary" }}>
          Startup, models, app identity, runs, runtimes and logs live in one CLI.{" "}
          <Box component="code" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.86em", color: "text.primary" }}>levier run</Box> defaults
          to the current app and default runtime, and watches the run to a terminal state.
        </Typography>

        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "320px minmax(0,1fr)" }, gap: { xs: 3, md: 6 }, alignItems: "start" }}>
          {desktop ? (
            <Tabs
              orientation="vertical"
              value={step}
              onChange={(event, next) => changeStep(next)}
              aria-label="CLI steps"
              sx={{ borderTop: 1, borderColor: "text.primary", "& .MuiTabs-indicator": { display: "none" } }}
            >
              {CLI_STEPS.map((item, index) => (
                <Tab
                  key={item.id}
                  value={index}
                  label={tabLabel(item)}
                  id={`cli-tab-${item.id}`}
                  aria-controls={`cli-panel-${item.id}`}
                  sx={{
                    minHeight: 68,
                    alignItems: "stretch",
                    justifyContent: "flex-start",
                    borderBottom: 1,
                    borderColor: "divider",
                    px: 1.5,
                    py: 1.25,
                    bgcolor: step === index ? "background.paper" : "transparent",
                    boxShadow: step === index ? (theme) => `inset 3px 0 0 ${theme.palette.primary.main}` : "none",
                    "&.Mui-selected": { bgcolor: "background.paper" }
                  }}
                />
              ))}
            </Tabs>
          ) : (
            <Tabs
              value={step}
              onChange={(event, next) => changeStep(next)}
              variant="scrollable"
              scrollButtons={false}
              aria-label="CLI steps"
              sx={{ borderBottom: 1, borderColor: "divider", mb: 2 }}
            >
              {CLI_STEPS.map((item, index) => (
                <Tab key={item.id} value={index} label={`${item.number} · ${item.title}`} id={`cli-tab-${item.id}`} aria-controls={`cli-panel-${item.id}`} />
              ))}
            </Tabs>
          )}

          <Box role="tabpanel" id={`cli-panel-${active.id}`} aria-labelledby={`cli-tab-${active.id}`} tabIndex={0} sx={{ minWidth: 0 }}>
            <Terminal step={active} animate={animate} />
            <Typography sx={{ mt: 1.5, fontSize: "0.71875rem", lineHeight: 1.65, color: "text.secondary" }}>
              IDs, versions and log lines are illustrative. Commands come from the repository QUICKSTART / USER_GUIDE.
              On Windows, use <Box component="code" sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.95em" }}>levier.exe</Box>.
            </Typography>
          </Box>
        </Box>
      </PageContainer>
    </Box>
  );
}

function ScopeSection() {
  return (
    <Box component="section" id="scope" sx={{ py: { xs: 6, md: 10 }, borderTop: 1, borderColor: "divider", scrollMarginTop: "84px" }}>
      <PageContainer>
        <SectionHeading index="04" title="Boundaries: who writes what" note="Runtimes are replaceable" />
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "repeat(3, minmax(0,1fr))" }, gap: { xs: 3.5, md: 4.5 } }}>
          {SCOPE.map((column) => (
            <Box key={column.title} sx={{ pt: 1.75, borderTop: "2px solid", borderColor: "text.primary" }}>
              <Typography sx={{ fontFamily: (theme) => theme.fonts.display, fontSize: "1.4375rem", fontWeight: 500, color: "text.primary" }}>
                {column.title}
              </Typography>
              <Stack component="ul" sx={{ listStyle: "none", mt: 1.5 }} spacing={0}>
                {column.items.map((item) => (
                  <Box
                    key={item}
                    component="li"
                    sx={{ display: "flex", alignItems: "baseline", gap: 1, py: 0.85, borderBottom: 1, borderColor: "divider", fontSize: "0.84375rem", color: "text.secondary" }}
                  >
                    <Box component="span" sx={{ width: 4, height: 4, bgcolor: "primary.main", flex: "none" }} />
                    {item}
                  </Box>
                ))}
              </Stack>
            </Box>
          ))}
        </Box>

        <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1.25, mt: 4, pt: 2.25, borderTop: 1, borderColor: "divider" }}>
          <Typography sx={{ fontSize: "0.71875rem", color: "text.secondary", mr: 0.5 }}>Handled by the SDK / CLI</Typography>
          {HANDLED_BY_SDK.map((item) => (
            <Chip key={item} label={item} variant="outlined" />
          ))}
        </Box>

        <Typography sx={{ maxWidth: "76ch", mt: 3.5, pt: 2, borderTop: 1, borderColor: "divider", fontSize: "0.84375rem", lineHeight: 1.75, color: "text.secondary" }}>
          All three are separate processes. Runtime and App depend only on the contract, so a runtime can be written
          your way — or replaced entirely.
        </Typography>
      </PageContainer>
    </Box>
  );
}

function RoadmapList({ label, items }) {
  return (
    <Box>
      <Typography sx={{ fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", letterSpacing: "0.07em", textTransform: "uppercase", color: "text.secondary" }}>
        {label}
      </Typography>
      <Stack component="ul" sx={{ listStyle: "none", mt: 1 }} spacing={0}>
        {items.map((item) => (
          <Box
            key={item}
            component="li"
            sx={{
              display: "flex",
              alignItems: "baseline",
              gap: 1,
              py: 0.85,
              borderBottom: 1,
              borderColor: "divider",
              fontSize: "0.84375rem",
              lineHeight: 1.6,
              color: "text.secondary"
            }}
          >
            <Box component="span" sx={{ width: 4, height: 4, bgcolor: "primary.main", flex: "none" }} />
            {item}
          </Box>
        ))}
      </Stack>
    </Box>
  );
}

function RoadmapSection() {
  return (
    <Box component="section" id="roadmap" sx={{ py: { xs: 6, md: 10 }, borderTop: 1, borderColor: "divider", scrollMarginTop: "84px" }}>
      <PageContainer>
        <SectionHeading index="05" title="Today and next" note="Scope" />
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: { xs: 4, md: 7 } }}>
          <RoadmapList label="Today" items={TODAY} />
          <RoadmapList label="Next" items={NEXT} />
        </Box>
        <Typography sx={{ maxWidth: "72ch", mt: 3.5, fontSize: "0.71875rem", lineHeight: 1.7, color: "text.secondary" }}>
          Python is the supported path on both sides today. The right-hand column is in progress.
        </Typography>
      </PageContainer>
    </Box>
  );
}

function DocsMapSection() {
  return (
    <Box component="section" id="documentation" sx={{ py: { xs: 6, md: 10 }, borderTop: 1, borderColor: "divider", scrollMarginTop: "84px" }}>
      <PageContainer>
        <SectionHeading index="06" title="Documentation map" note="6 layers" />
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            borderTop: 1,
            borderColor: "text.primary"
          }}
        >
          {DOC_MAP.map((entry, index) => (
            <ButtonBase
              key={entry.index}
              component="a"
              href={entry.href}
              sx={{
                display: "grid",
                gridTemplateColumns: "42px minmax(0,1fr) 18px",
                alignItems: "start",
                gap: 1.75,
                p: 2.5,
                pr: { sm: index % 2 === 0 ? 3.25 : 2.5 },
                pl: { sm: index % 2 === 0 ? 2.5 : 3.25 },
                borderBottom: 1,
                borderRight: { sm: index % 2 === 0 ? 1 : 0 },
                borderColor: "divider",
                textAlign: "left",
                transition: "background-color 160ms ease",
                "&:hover": { bgcolor: "action.hover" },
                "&:active": { bgcolor: "action.selected" },
                "&:focus-visible": { outline: "2px solid", outlineColor: "primary.main", outlineOffset: -2 }
              }}
            >
              <Typography sx={{ pt: 0.25, fontFamily: (theme) => theme.fonts.mono, fontSize: "0.65625rem", color: "primary.main" }}>
                {entry.index}
              </Typography>
              <Box>
                <Typography sx={{ fontSize: "0.96875rem", fontWeight: 600, color: "text.primary" }}>{entry.title}</Typography>
                <Typography sx={{ mt: 0.5, fontSize: "0.78125rem", lineHeight: 1.65, color: "text.secondary" }}>{entry.description}</Typography>
              </Box>
              <ArrowUpRight size={17} style={{ color: "currentColor", opacity: 0.7, marginTop: 2 }} />
            </ButtonBase>
          ))}
        </Box>

        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 2,
            mt: 3.5,
            pt: 2.5,
            borderTop: 1,
            borderColor: "divider"
          }}
        >
          <Typography sx={{ fontSize: "0.71875rem", color: "text.secondary" }}>
            More:{" "}
            <Link href="./docs/index.html#/sdk" underline="always" sx={{ color: "text.primary", textDecorationColor: "divider" }}>SDK</Link>{" "}
            ·{" "}
            <Link href="./docs/index.html#/env" underline="always" sx={{ color: "text.primary", textDecorationColor: "divider" }}>Environment</Link>{" "}
            ·{" "}
            <Link href="./docs/index.html#/errors" underline="always" sx={{ color: "text.primary", textDecorationColor: "divider" }}>Troubleshooting</Link>{" "}
            ·{" "}
            <Link href="./docs/index.html#/glossary" underline="always" sx={{ color: "text.primary", textDecorationColor: "divider" }}>Glossary</Link>
          </Typography>
          <Button component="a" href="./docs/" variant="contained" endIcon={<ArrowRight size={15} />}>
            Open the docs
          </Button>
        </Box>
      </PageContainer>
    </Box>
  );
}

export function LandingApp() {
  const activeSection = useActiveSection(SECTION_IDS);

  return (
    <Box id="top" sx={{ minHeight: "100vh", display: "flex", flexDirection: "column" }}>
      <SkipLink href="#main">Skip to main content</SkipLink>
      <LandingHeader activeSection={activeSection} />
      <Box component="main" id="main" sx={{ flex: 1 }}>
        <Hero />
        <QuickInstall />
        <RuntimeSection />
        <EndpointsSection />
        <CliSection />
        <ScopeSection />
        <RoadmapSection />
        <DocsMapSection />
      </Box>
      <SiteFooter />
    </Box>
  );
}
