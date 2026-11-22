import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Drawer, useMediaQuery } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import { useReducedMotion } from "motion/react";
import { DocsHeader } from "../components/DocsHeader";
import { DocsFooter } from "../components/DocsFooter";
import { SkipLink } from "../components/SkipLink";
import { docPresentation, sectionLabelFor } from "../content/nav.en";
import { DOCS_COPY } from "../content/docsCopy";
import { parseFrontmatter } from "../lib/markdown";
import { DocArticle } from "./DocArticle";
import { DocEmpty } from "./DocEmpty";
import { DocError } from "./DocError";
import { DocSkeleton } from "./DocSkeleton";
import { DocsSidebar } from "./DocsSidebar";
import { DocToc } from "./DocToc";

const SLUG_ALIASES = {
  overview: "onboarding",
  quickstart: "onboarding",
  install: "levier-install",
  concepts: "onboarding",
  "app-sdk": "app-core",
  "runtime-sdk": "runtime-core",
  architecture: "levier-install",
  troubleshooting: "errors",
  app: "app-core",
  runtime: "runtime-core",
  stack: "levier-install",
  status: "levier-status",
  catalog: "pre-model",
  "run-admin": "levier-run",
  model: "pre-model",
  mcp: "pre-mcp",
  config: "pre-env",
  secrets: "pre-env",
  "pre-model-check": "pre-model",
  "pre-mcp-config": "pre-mcp",
  "pre-project": "pre-profile",
  "pre-secrets": "pre-env",
  "app-run": "app-runs",
  "app-tools": "app-install",
  "app-app": "app-install",
  "app-init": "app-dev",
  "app-run-cli": "app-runs",
  "app-run-sdk": "app-runs",
  "app-errors": "app-mcp",
  "app-check": "app-manage",
  "levier-start": "levier-install",
  "levier-setup": "levier-install",
  "levier-down": "levier-status",
  "levier-logs": "levier-status",
  "levier-run-show": "levier-run",
  "levier-config": "levier-doctor",
  "runtime-install": "runtime-start",
  "runtime-init": "runtime-start",
  "runtime-checkpoint": "runtime-errors",
  "runtime-list": "runtime-remote",
  "runtime-ops": "runtime-remote"
};

function flatten(manifest) {
  const flat = [];
  (manifest.sections || []).forEach((section) => {
    (section.items || []).forEach((item) => {
      flat.push({
        slug: item.slug,
        title: item.title,
        status: item.status || "published",
        file: item.file,
        description: item.description || "",
        section: section.title || ""
      });
    });
  });
  return flat;
}

function readHash() {
  try {
    return decodeURIComponent(window.location.hash.replace(/^#\/?/, "").trim().split("/")[0] || "");
  } catch (error) {
    return "";
  }
}

const LANGUAGE_KEY = "levier-docs-language";

const SECTION_START = {
  "接入": "onboarding",
  "配置": "pre-model",
  "App 端接入与管理": "app-core",
  "Levier 端管理": "levier-install",
  "Runtime 端接入与管理": "runtime-core",
  "总览": "cli"
};

function readLocale() {
  try {
    return window.localStorage.getItem(LANGUAGE_KEY) === "zh" ? "zh" : "en";
  } catch (error) {
    return "en";
  }
}

function fileForLocale(item, locale) {
  return locale === "en" ? item.file.replace(/^content\//, "content/en/") : item.file;
}

export function DocsApp() {
  const theme = useTheme();
  const reduced = useReducedMotion();
  const desktopSidebar = useMediaQuery(theme.breakpoints.up("md"));
  const desktopToc = useMediaQuery(theme.breakpoints.up("lg"));

  const [locale, setLocale] = useState(readLocale);
  const copy = DOCS_COPY[locale];
  const [manifest, setManifest] = useState(null);
  const [flat, setFlat] = useState([]);
  const [manifestState, setManifestState] = useState("loading");
  const [manifestError, setManifestError] = useState(null);
  const [manifestToken, setManifestToken] = useState(0);
  const [requested, setRequested] = useState(readHash);
  const [slug, setSlug] = useState("");
  const [doc, setDoc] = useState(null);
  const [docState, setDocState] = useState("idle");
  const [docError, setDocError] = useState(null);
  const [retryToken, setRetryToken] = useState(0);
  const [headings, setHeadings] = useState([]);
  const [activeHeading, setActiveHeading] = useState("");
  const [tocOpen, setTocOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [query, setQuery] = useState("");

  const mainRef = useRef(null);
  const menuRef = useRef(null);
  const manifestURL = useMemo(() => new URL("manifest.json", window.location.href), []);

  const toggleLocale = useCallback(() => {
    const next = locale === "en" ? "zh" : "en";
    setLocale(next);
    try {
      window.localStorage.setItem(LANGUAGE_KEY, next);
    } catch (error) {
      /* The preference is optional. */
    }
  }, [locale]);

  useEffect(() => {
    document.documentElement.lang = copy.htmlLang;
    document.querySelector('meta[name="description"]')?.setAttribute("content", copy.metaDescription);
  }, [copy.htmlLang, copy.metaDescription]);

  const item = useMemo(() => flat.find((entry) => entry.slug === slug), [flat, slug]);
  const activeSectionSlug = item ? SECTION_START[item.section] || item.slug : "";
  const previous = useMemo(() => {
    if (!item) return null;
    const index = flat.indexOf(item);
    return index > 0 ? flat[index - 1] : null;
  }, [flat, item]);

  const next = useMemo(() => {
    if (!item) return null;
    const index = flat.indexOf(item);
    return index >= 0 && index < flat.length - 1 ? flat[index + 1] : null;
  }, [flat, item]);

  const loadManifest = useCallback(() => {
    setManifestState("loading");
    setManifestError(null);
    fetch(manifestURL, { cache: "no-store" })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status} · manifest.json`);
        return response.json();
      })
      .then((data) => {
        const entries = flatten(data);
        setManifest(data);
        setFlat(entries);
        setManifestState(entries.length ? "ready" : "empty");
      })
      .catch((error) => {
        setManifestError(error.message);
        setManifestState("error");
      });
  }, [manifestURL]);

  useEffect(() => {
    loadManifest();
  }, [loadManifest, manifestToken]);

  useEffect(() => {
    const onHash = () => setRequested(readHash());
    window.addEventListener("hashchange", onHash);
    onHash();
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  useEffect(() => {
    if (!flat.length) return;
    const alias = SLUG_ALIASES[requested] || requested;
    const target = flat.find((entry) => entry.slug === alias) || flat[0];
    if (target.slug !== requested) {
      history.replaceState(null, "", `#/${target.slug}`);
    }
    setSlug(target.slug);
  }, [flat, requested]);

  useEffect(() => {
    if (!item) return undefined;
    const controller = new AbortController();
    const file = fileForLocale(item, locale);
    setDocState("loading");
    setDocError(null);
    setHeadings([]);
    setActiveHeading("");
    setTocOpen(false);
    setDrawerOpen(false);
    window.scrollTo({ top: 0, behavior: "auto" });

    fetch(new URL(file, manifestURL), { cache: "no-store", signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status} · ${file}`);
        return response.text();
      })
      .then((raw) => {
        const parsed = parseFrontmatter(raw);
        setDoc(parsed);
        setDocState("ready");
      })
      .catch((error) => {
        if (error.name === "AbortError") return;
        setDocError(error);
        setDocState("error");
      });

    return () => controller.abort();
  }, [item, locale, manifestURL, retryToken]);

  useEffect(() => {
    if (docState !== "ready" || !item) return undefined;
    const timeout = window.setTimeout(() => {
      mainRef.current?.focus({ preventScroll: true });
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [docState, item]);

  useEffect(() => {
    if (item) {
      document.title = copy.docsTitle(docPresentation(item, locale).title);
    }
  }, [item, locale, copy]);

  useEffect(() => {
    if (headings.length && !headings.some((heading) => heading.id === activeHeading)) {
      setActiveHeading(headings[0].id);
    }
    // The first heading becomes the initial TOC state; scrolling takes over afterwards.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [headings]);

  useEffect(() => {
    if (!headings.length || docState !== "ready" || !("IntersectionObserver" in window)) return undefined;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setActiveHeading(visible[0].target.id);
      },
      { rootMargin: "-18% 0px -68% 0px", threshold: 0 }
    );
    headings.forEach((heading) => {
      const element = document.getElementById(heading.id);
      if (element) observer.observe(element);
    });
    return () => observer.disconnect();
  }, [headings, docState]);

  const handleTocNavigate = useCallback(
    (id) => {
      const element = document.getElementById(id);
      if (!element) return;
      setActiveHeading(id);
      element.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "start" });
    },
    [reduced]
  );

  const handleDrawerClose = (event, reason) => {
    setDrawerOpen(false);
    if (reason === "backdropClick" || reason === "escapeKeyDown") {
      menuRef.current?.focus();
    }
  };

  const handleDrawerButton = () => {
    setDrawerOpen(false);
    menuRef.current?.focus();
  };

  const sidebar = (
    <DocsSidebar
      sections={manifest?.sections || []}
      activeSlug={slug}
      query={query}
      onQuery={setQuery}
      onNavigate={() => setDrawerOpen(false)}
      status={manifestState}
      error={manifestError}
      onRetry={() => setManifestToken((value) => value + 1)}
      total={flat.length}
      onClose={desktopSidebar ? undefined : handleDrawerButton}
      locale={locale}
      copy={copy}
    />
  );

  const showSkeleton =
    manifestState === "loading" ||
    (manifestState === "ready" && (docState === "loading" || docState === "idle" || !item));
  const showManifestError = manifestState === "error";
  const showManifestEmpty = manifestState === "empty";
  const showDocError = docState === "error";
  const showDoc = docState === "ready" && doc && item;

  return (
    <Box sx={{ minHeight: "100vh", display: "flex", flexDirection: "column", bgcolor: "background.default" }}>
      <SkipLink href="#doc-main">{copy.skipDocs}</SkipLink>

      <DocsHeader
        onOpenNav={() => setDrawerOpen(true)}
        menuButtonRef={menuRef}
        activeSection={activeSectionSlug}
        navOpen={drawerOpen}
        locale={locale}
        onToggleLocale={toggleLocale}
        copy={copy}
      />

      <Box
        sx={{
          flex: 1,
          width: "100%",
          maxWidth: 1360,
          mx: "auto",
          px: { xs: 2, md: 3 },
          py: { xs: 2.5, md: 3.5 },
          pb: { xs: 6, md: 8 },
          display: "grid",
          gridTemplateColumns: {
            xs: "minmax(0, 1fr)",
            md: "220px minmax(0, 1fr)",
            lg: "220px minmax(0, 1fr) 180px",
            xl: "250px minmax(0, 760px) 200px"
          },
          columnGap: { xs: 0, md: 4, xl: 5 },
          alignItems: "start",
          justifyContent: "center"
        }}
      >
        {desktopSidebar ? (
          <Box
            component="aside"
            sx={{
              position: "sticky",
              top: 84,
              maxHeight: "calc(100vh - 104px)",
              overflowY: "auto",
              pr: 2,
              borderRight: 1,
              borderColor: "divider",
              alignSelf: "start"
            }}
          >
            {sidebar}
          </Box>
        ) : null}

        <Box
          component="main"
          id="doc-main"
          ref={mainRef}
          tabIndex={-1}
          data-focus-silent
          sx={{ minWidth: 0, outline: "none" }}
        >
          {showSkeleton ? <DocSkeleton copy={copy} /> : null}
          {showManifestError ? (
            <DocError
              item={null}
              error={manifestError}
              title={copy.error.manifestTitle}
              fileLabel="manifest.json"
              hint={copy.error.manifestHint}
              onRetry={() => setManifestToken((value) => value + 1)}
              copy={copy}
            />
          ) : null}
          {showManifestEmpty ? <DocEmpty copy={copy} /> : null}
          {showDocError ? (
            <DocError
              item={item}
              error={docError}
              onRetry={() => setRetryToken((value) => value + 1)}
              copy={copy}
            />
          ) : null}
          {showDoc ? (
            <DocArticle
              item={item}
              meta={doc.meta}
              body={doc.body}
              onHeadings={setHeadings}
              previous={previous}
              next={next}
              activeHeading={activeHeading}
              onTocNavigate={handleTocNavigate}
              tocExpanded={tocOpen}
              onTocChange={(event, expanded) => setTocOpen(expanded)}
              showMobileToc={!desktopToc}
              slug={slug}
              locale={locale}
              copy={copy}
            />
          ) : null}
        </Box>

        {desktopToc && headings.length > 0 ? (
          <Box
            component="aside"
            aria-label={copy.toc.aria}
            sx={{
              position: "sticky",
              top: 84,
              maxHeight: "calc(100vh - 104px)",
              overflowY: "auto",
              pl: 2,
              borderLeft: 1,
              borderColor: "divider",
              alignSelf: "start"
            }}
          >
            <DocToc
              headings={headings}
              activeId={activeHeading}
              slug={slug}
              onNavigate={handleTocNavigate}
              copy={copy}
            />
          </Box>
        ) : null}
      </Box>

      <DocsFooter copy={copy} />

      <Drawer
        anchor="left"
        open={drawerOpen}
        onClose={handleDrawerClose}
        slotProps={{
          paper: {
            sx: {
              width: 320,
              maxWidth: "88vw",
              borderRight: 1,
              borderColor: "text.primary",
              bgcolor: "background.default"
            }
          }
        }}
      >
        <Box sx={{ p: 2.25, pb: 4 }}>{sidebar}</Box>
      </Drawer>

      <Box
        aria-live="polite"
        sx={{ position: "absolute", width: "1px", height: "1px", overflow: "hidden", clip: "rect(0 0 0 0)" }}
      >
        {item ? copy.routeAnnounce(docPresentation(item, locale).title, sectionLabelFor(item.section, locale)) : ""}
      </Box>
    </Box>
  );
}
