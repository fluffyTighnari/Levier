# Levier — website

The public Levier site: a React 19 + Material UI v9 application built with Vite.

- The landing page is English.
- Documentation is bilingual: English is the default, and an **EN / 中文** button switches the whole documentation shell and the page content.
- Every documentation page has a Chinese source in `docs/content/` and an English mirror in `docs/content/en/`.
- `docs/manifest.json` remains the structural source of truth; English titles and descriptions live in `src/content/nav.en.js`.
- The language preference is stored locally and does not change routes.

## Current support

- Python App SDK
- Python Runtime SDK
- Local CLI control plane

The landing page also shows what comes next: TypeScript, Java and Go SDKs; sandbox and long-running runtimes; multi-agent runs; and streaming. Those surfaces are listed as upcoming, not shipped.

## Quick start

```bash
cd site
npm install
npm run dev
```

Open:

```text
http://127.0.0.1:5173/            landing page
http://127.0.0.1:5173/docs/       documentation shell
```

The documentation pages use hash routes, for example:

```text
http://127.0.0.1:5173/docs/#/onboarding
http://127.0.0.1:5173/docs/#/pre-model
```

## Build and deploy

```bash
cd site
npm run build
```

The build runs the content integrity check first, then Vite, then copies the Markdown source into the output.

Deploy the generated `site/dist/` directory. The source `site/` directory is not the deployable artifact anymore.

Preview the production build locally:

```bash
python -m http.server 4173 --directory site/dist
```

Open `http://127.0.0.1:4173/` and `http://127.0.0.1:4173/docs/`.

## Structure

```text
site/
  index.html                  landing entry
  docs/
    index.html                docs entry
    manifest.json             navigation structure (source of truth)
    content/*.md              31 Chinese documents (source of truth)
    content/en/*.md           31 English mirrors
  src/
    main-landing.jsx
    main-docs.jsx
    theme/
      tokens.js               the only colour literals in the UI source
      index.js                MUI theme and component overrides
    components/               shared header, footer, brand, code surface, terminal
    landing/                  landing page sections
    docs/                     docs shell, sidebar, article, TOC, states
    content/
      nav.en.js               English titles and descriptions
      docsCopy.js             bilingual documentation-shell labels
      landing.js              landing copy, samples and CLI trace data
    lib/
      markdown.jsx            marked token renderer to MUI components
      clipboard.js
      useActiveSection.js
  scripts/
    verify-content.mjs        manifest, file and link integrity check
    copy-content.mjs          copies Markdown into dist/docs
```

## Content model

`docs/manifest.json` defines six sections and 31 pages. Every page points at a Chinese Markdown file under `docs/content/` and has an English mirror with the same filename under `docs/content/en/`.

`src/content/nav.en.js` maps each slug to an English title and description for navigation, search, the document header and the pager. `src/content/docsCopy.js` holds the bilingual shell labels.

The docs loader selects the file by language:

```text
English   content/en/<file>.md
Chinese   content/<file>.md
```

To add a page:

1. Copy `docs/content/_template.md` to a new file under `docs/content/`.
2. Copy `docs/content/en/_template.md` to the same filename under `docs/content/en/`.
3. Add an entry to `docs/manifest.json` with the Chinese file path.
4. Add the English title and description to `src/content/nav.en.js`.
5. Run `npm run verify:content` and `npm run build`.

Markdown support:

- GFM tables are rendered with MUI `Table`.
- Fenced code blocks get a language label and copy button.
- Callouts (`> [!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]`, `[!CAUTION]`, `[!OUTPUT]`, `[!FAIL]`) become MUI `Alert` variants.
- `.md` links become hash routes.
- External links open in a new tab.
- `##` headings generate the TOC and stable anchors.
- Every code block has a visible Copy action. The Linux/macOS install command is a single copyable line; Windows PowerShell instructions stay on the same install page. There are no OS-specific document files.

## Language

- The first visit opens in English.
- The language button in the docs top bar switches between `EN` and `中文`.
- The preference is stored in `localStorage` under `levier-docs-language`.
- Switching language reloads the current document, the sidebar labels, search, TOC title, pager, callouts, copy feedback and all loading/error/empty states.
- Routes and aliases do not change. A shared link opens in the reader's stored language.

## Routes

The docs shell preserves the existing hash routes and aliases. For example:

```text
overview / quickstart / concepts  → onboarding
install                           → levier-install
app / app-sdk                     → app-core
app-run                           → app-run-cli
runtime / runtime-sdk             → runtime-core
architecture / stack              → levier-install
troubleshooting                   → errors
model / catalog                   → pre-model
mcp                               → pre-mcp
config                            → pre-env
secrets                           → pre-env
pre-model-check                   → pre-model
pre-mcp-config                    → pre-mcp
pre-project                       → pre-profile
pre-secrets                       → pre-env
app-tools / app-app               → app-install
app-init                          → app-dev
app-run-cli / app-run-sdk         → app-runs
app-errors                        → app-mcp
app-check                         → app-manage
levier-start / levier-setup       → levier-install
levier-down / levier-logs         → levier-status
levier-run-show                   → levier-run
levier-config                     → levier-doctor
runtime-install / runtime-init    → runtime-start
runtime-checkpoint                → runtime-errors
runtime-list                      → runtime-remote
```

Unknown routes go to the first document. Page titles update `document.title`; focus moves to the main region without a visible focus ring; the route is announced to assistive technology.

## Design system

All colours are defined in `src/theme/tokens.js` and consumed through the MUI theme. Components do not contain colour literals.

- Canvas: warm paper
- Ink: near-black
- Accent: rust, used for emphasis and primary actions only
- Status: success, warning, error and info only mean status
- Type: Newsreader for display, IBM Plex Sans / Noto Sans SC for reading, IBM Plex Mono for code
- Shape: 6px controls, 10px surfaces, 14px large containers
- Borders: 1px rules; no gradients, glass blur or large shadows

The landing page and the documentation shell share the same theme, brand, focus treatment and code surfaces.

## States and accessibility

- Language: English default; EN / 中文 toggle; the preference is stored locally.
- Documentation navigation: loading skeleton, manifest error with retry, empty manifest state, no-results state for the filter.
- Document body: loading skeleton, fetch error with retry, empty state.
- Copy actions: success and failure feedback with an accessible live region.
- Tabs: MUI keyboard navigation with arrow, Home and End keys.
- Mobile navigation: MUI Drawer with focus trap, Escape close and focus return.
- The mobile TOC is a collapsed MUI Accordion.
- Motion is limited to state changes and respects `prefers-reduced-motion`.

## Verification

```bash
npm run verify:content
npm run build
```

`verify:content` checks:

- all manifest files exist in both `content/` and `content/en/`
- slugs are unique
- frontmatter titles exist
- English titles and descriptions match `nav.en.js`
- `status` and `updated` match between languages
- heading, fenced-block and callout counts match between languages
- internal `.md` links resolve in both languages
- no document body is empty
- no Chinese characters remain in an English document

The site has no separate unit-test runner. The project's Go test suite is independent of the website and runs with:

```bash
go test ./...
```

## Relationship to the repository docs

`site/docs/content/` is the public-facing documentation. Internal SCOPE, design, ADR and report files remain under the repository's `docs/` directory and are not part of this site.
