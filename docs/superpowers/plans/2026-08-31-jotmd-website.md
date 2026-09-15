# JotMD Website Implementation Plan

**Status:** completed on 2026-08-31 in `d560410`.

## Final implementation

- GitHub Pages publishes the dependency-free static site from `site/` through
  `.github/workflows/pages.yml`.
- `site/index.html` and `site/docs.html` share the vault-map layout and use
  `site/styles.css` plus progressive enhancement from `site/site.js`.
- Product screenshots and the favicon live in `site/assets/`.
- `site/llms.txt` contains the complete LLM-facing documentation;
  `llms-full.txt` was deliberately removed as redundant.
- `internal/sitecheck/site_test.go` validates pages, assets, local links,
  anchors, sitemap, robots, and LLM discovery. `site/site.test.cjs` covers the
  scroll-aware vault navigation.
- The final palette uses JotMD green `#77ad91` and gold `#e1b866`.

The unchecked steps below preserve the original execution history. Do not run
them as outstanding work: paths and several decisions changed during visual
iteration. The final implementation above and the current code are
authoritative.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a distinctive English landing page and detailed documentation page for the JotMD terminal TUI.

**Architecture:** Use hand-written static HTML, shared CSS, and progressively enhanced JavaScript. GitHub Pages serves `site/`; relative URLs keep the site valid beneath the repository path and under a local HTTP server. Go integration tests serve the real `site/` tree and verify pages, local assets, and anchors without adding dependencies.

**Tech Stack:** HTML5, CSS, browser JavaScript, Go standard-library integration test, GitHub Pages

**Spec:** `docs/superpowers/specs/2026-08-31-jotmd-website-design.md`

## Global Constraints

- Keep the site dependency-free: no package manager, framework, generated build output, external font, analytics, or new dependency.
- Publish from `site/`; all internal URLs and asset paths must be relative and work below `/jotmd/`.
- Present JotMD first as a keyboard-first Markdown TUI for the terminal.
- Present `jot-memory` as an optional secondary feature for Codex, Claude Code, and OpenCode.
- Use `README.md`, `docs/jot-memory.md`, runtime help, configuration templates, and keymap definitions as the source of truth; do not invent commands.
- Keep screenshots and the favicon in `site/assets/`.
- Preserve the approved terminal-first visual direction: near-black surfaces, system monospace, JotMD green `#77ad91`, gold `#e1b866`, and restrained borders.
- Keep the pages useful without JavaScript and honor `prefers-reduced-motion`.

---

### Task 1: Static-site contract and landing page

**Files:**
- Create: `internal/sitecheck/site_test.go`
- Create: `docs/index.html`
- Create: `docs/styles.css`
- Create: `docs/site.js`
- Create: `docs/.nojekyll`

**Interfaces:**
- Consumes: `docs/images/jotmd.png`, existing install command and feature descriptions from `README.md`
- Produces: shared classes and tokens in `docs/styles.css`; `data-copy` controls handled by `docs/site.js`; HTTP-level static-site test coverage

- [ ] **Step 1: Add the failing landing-page integration test**

Create `internal/sitecheck/site_test.go`. Start `httptest.NewServer` with
`http.FileServer(http.Dir("../../docs"))`, request `/`, and require status 200.
Extract relative `href` and `src` values with a small regexp, skip external and
fragment-only links, request non-HTML assets, and require status 200. This test
catches a missing landing page, stylesheet, script, image, or broken relative
asset path while exercising the actual static server.

- [ ] **Step 2: Run the contract and verify it fails**

Run: `go test ./internal/sitecheck -run TestLandingPageServesLocalAssets -count=1`

Expected: FAIL because requesting `/` returns a directory listing without the
required page assets.

- [ ] **Step 3: Create the landing-page markup**

Create semantic HTML with this exact high-level order:

```html
<body>
  <a class="skip-link" href="#main">Skip to content</a>
  <header class="site-header">...</header>
  <main id="main">
    <section class="hero">...</section>
    <section id="features" class="section features">...</section>
    <section class="section workflow">...</section>
    <section id="agent-memory" class="section memory">...</section>
    <section id="install" class="section install">...</section>
  </main>
  <footer class="site-footer">...</footer>
  <script src="site.js"></script>
</body>
```

Required hero copy and actions:

```html
<p class="eyebrow">// Markdown notes, zero ceremony</p>
<h1>Your notes.<br><span>Right where</span><br>you work.</h1>
<p class="hero-copy">JotMD is a keyboard-first Markdown TUI for your terminal. Browse plain files, search your vault, and preview Markdown without leaving the command line.</p>
<pre class="command"><code>brew install gonfff/tap/jotmd</code><button type="button" data-copy="brew install gonfff/tap/jotmd" aria-label="Copy install command">Copy</button></pre>
<a class="button secondary" href="docs.html">Read the docs</a>
```

Use `images/jotmd.png` with descriptive alt text. Add six capability cards, the
three-step terminal workflow, the `recall → work → capture` memory flow, the
agent-memory visibility note, installation CTA, GitHub link, and MIT link from
the spec. Use current product facts from `README.md` only.

- [ ] **Step 4: Create the visual system**

Define shared tokens and core layout in `docs/styles.css`:

```css
:root {
  --ink: #0c0e12;
  --surface: #181b21;
  --surface-raised: #242830;
  --line: #464d58;
  --paper: #f3efe7;
  --muted: #aeb3bd;
  --mint: #99d58f;
  --coral: #ef8d8a;
  --yellow: #d8b75e;
  --sans: Arial, Helvetica, sans-serif;
  --mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  --content: 72rem;
}
```

Implement the approved asymmetric hero, hard two-pixel borders, offset shadows,
large uppercase headline, terminal screenshot treatment, feature grid, workflow,
memory flow, documentation layout primitives, responsive breakpoints at 900px
and 640px, `:focus-visible`, skip link, code/table overflow, and:

```css
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    scroll-behavior: auto !important;
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
  }
}
```

- [ ] **Step 5: Add progressive copy controls**

Create `docs/site.js` with one delegated click handler so both pages can reuse it:

```js
document.addEventListener("click", async (event) => {
  const button = event.target.closest("[data-copy]");
  if (!button) return;

  await navigator.clipboard.writeText(button.dataset.copy);
  const label = button.textContent;
  button.textContent = "Copied";
  button.setAttribute("aria-label", "Command copied");
  setTimeout(() => {
    button.textContent = label;
    button.setAttribute("aria-label", "Copy command");
  }, 1500);
});
```

Add an empty `docs/.nojekyll` file.

- [ ] **Step 6: Run the landing-page integration test**

Run: `go test ./internal/sitecheck -run TestLandingPageServesLocalAssets -count=1`

Expected: PASS. The landing page and each referenced CSS, JavaScript, and image
asset return HTTP 200.

- [ ] **Step 7: Commit the landing page**

```sh
git add internal/sitecheck/site_test.go docs/.nojekyll docs/index.html docs/styles.css docs/site.js
git commit -m "feat: add JotMD product landing page"
```

### Task 2: Detailed documentation page

**Files:**
- Modify: `internal/sitecheck/site_test.go`
- Create: `docs/docs.html`
- Reuse: `docs/styles.css`, `docs/site.js`

**Interfaces:**
- Consumes: shared header, typography, code, table, button, and documentation layout classes from `docs/styles.css`
- Produces: `docs.html#agent-memory` target used by the landing page and complete static product documentation

- [ ] **Step 1: Add and run the failing documentation integration test**

Extend `internal/sitecheck/site_test.go` with
`TestDocumentationServesLocalLinksAndAnchors`. Request `/docs.html`, follow
relative file links, and verify each fragment matches an `id` in the target
document.

Run: `go test ./internal/sitecheck -run TestDocumentationServesLocalLinksAndAnchors -count=1`

Expected: FAIL because `/docs.html` is missing.

- [ ] **Step 2: Create the documentation shell and navigation**

Use the same `<head>`, header, footer, stylesheet, and script as the landing page.
The main structure is:

```html
<main id="main" class="docs-layout">
  <aside class="docs-tree" aria-label="Documentation sections">
    <nav>...</nav>
  </aside>
  <article class="docs-content">
    <h1>Documentation</h1>
    <section id="requirements">...</section>
    <section id="installation">...</section>
    <section id="quick-start">...</section>
    <section id="using-the-tui">...</section>
    <section id="essential-keys">...</section>
    <section id="configuration">...</section>
    <section id="cli">...</section>
    <section id="agent-memory">...</section>
    <section id="development">...</section>
  </article>
</main>
```

Group the sidebar links under `Getting started`, `TUI`, `Automation`, and
`Project`. Prefix active-looking tree labels with small `›`, `#`, or file glyphs
using text/CSS rather than hand-written SVG artwork.

- [ ] **Step 3: Add verified TUI documentation**

Transcribe the current commands and behavior from `README.md`:

```text
brew install gonfff/tap/jotmd
mkdir -p ~/vault
jotmd --notes-dir ~/vault
jotmd --dump-keys
jotmd --init-themes
jotmd --list-themes
jotmd --dump-config
jotmd config check [PATH]
```

Include the current essential-key table, note-management capabilities, config
precedence, editor fallback, theme behavior, and development commands. Use copy
controls only for commands users are likely to run directly.

- [ ] **Step 4: Add verified CLI documentation**

Document these exact forms and the safe revision workflow from `README.md`:

```text
jotmd [--notes-dir PATH] search [--limit N] QUERY [--json]
jotmd [--notes-dir PATH] get PATH [--json]
jotmd [--notes-dir PATH] write [--if-revision REVISION] PATH [--json]
jotmd [--notes-dir PATH] delete --if-revision REVISION PATH [--json]
```

State that create refuses to overwrite, update/delete require the returned
revision, JSON success uses stdout, errors use stderr, and delete is permanent.

- [ ] **Step 5: Add verified agent-memory documentation**

Use `docs/jot-memory.md` as the source for prerequisites, install, update,
removal, and verification. Include all three supported hosts, the storage tree,
the `a` visibility key, `show_agent_memory = true`, and the reserved-directory
warning. Commands must match the source exactly, including:

```text
codex plugin marketplace add gonfff/jotmd
codex plugin add jot-memory@jotmd

claude plugin marketplace add gonfff/jotmd
claude plugin install jot-memory@jotmd

git clone --depth 1 https://github.com/gonfff/jotmd.git ~/.local/share/jotmd
ln -s ~/.local/share/jotmd/skills/jot-memory ~/.config/opencode/skills/jot-memory
```

- [ ] **Step 6: Run the site integration tests**

Run: `go test ./internal/sitecheck -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the documentation page**

```sh
git add internal/sitecheck/site_test.go docs/docs.html
git commit -m "docs: add detailed JotMD website guide"
```

### Task 3: Crawler and LLM discovery files

**Files:**
- Modify: `internal/sitecheck/site_test.go`
- Modify: `docs/index.html`
- Modify: `docs/docs.html`
- Create: `docs/robots.txt`
- Create: `docs/sitemap.xml`
- Create: `docs/llms.txt`
- Create: `docs/llms-full.txt`

**Interfaces:**
- Consumes: final public base URL `https://gonfff.github.io/jotmd/` and verified documentation content
- Produces: search crawler discovery, XML sitemap, concise LLM index, and self-contained LLM context

- [ ] **Step 1: Add and run the failing discovery-file test**

Add `TestDiscoveryFiles` to `internal/sitecheck/site_test.go`. Request the four
files from the real static server, decode `sitemap.xml` with `encoding/xml`, and
verify it contains the absolute landing and docs URLs. Require the robots sitemap
pointer, an H1 and absolute documentation links in `llms.txt`, and the core
`Installation`, `Agent CLI`, and `Agent memory` sections in `llms-full.txt`.

Run: `go test ./internal/sitecheck -run TestDiscoveryFiles -count=1`

Expected: FAIL because `robots.txt`, `sitemap.xml`, `llms.txt`, and
`llms-full.txt` do not exist.

- [ ] **Step 2: Create crawler files**

Create `docs/robots.txt`:

```text
User-agent: *
Allow: /

Sitemap: https://gonfff.github.io/jotmd/sitemap.xml
```

Create a valid sitemap URL set containing:

```text
https://gonfff.github.io/jotmd/
https://gonfff.github.io/jotmd/docs.html
```

- [ ] **Step 3: Create LLM discovery files**

Create `llms.txt` with the current proposal's H1, summary blockquote, short
product context, and grouped absolute links to the homepage, documentation,
agent-memory anchor, full context, source, and license.

Create `llms-full.txt` as self-contained Markdown covering the verified product
description, installation, quick start, TUI capabilities and keys,
configuration/themes, safe Agent CLI revision workflow, and agent-memory setup
for Codex, Claude Code, and OpenCode. Do not add claims or commands absent from
the repository sources.

- [ ] **Step 4: Link the LLM index from both HTML pages**

Add this to each `<head>` using the correct relative path from that page:

```html
<link rel="alternate" type="text/markdown" href="llms.txt" title="JotMD documentation for language models">
```

- [ ] **Step 5: Run the discovery and complete site tests**

Run:

```sh
go test ./internal/sitecheck -run TestDiscoveryFiles -count=1
go test ./internal/sitecheck -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit discovery files**

```sh
git add internal/sitecheck/site_test.go docs/index.html docs/docs.html docs/robots.txt docs/sitemap.xml docs/llms.txt docs/llms-full.txt
git commit -m "docs: add crawler and LLM discovery files"
```

### Task 4: HTTP and repository validation

**Files:**
- Modify only if validation finds a real defect: `docs/index.html`, `docs/docs.html`, `docs/styles.css`, `docs/site.js`, `internal/sitecheck/site_test.go`

**Interfaces:**
- Consumes: complete static site from Tasks 1 and 2
- Produces: GitHub Pages-ready files with verified local routing and repository checks

- [ ] **Step 1: Check formatting and the static contract**

Run:

```sh
git diff --check
go test ./internal/sitecheck -count=1
```

Expected: both commands exit 0.

- [ ] **Step 2: Serve the exact GitHub Pages source directory**

Run a temporary local server from `docs/`:

```sh
python3 -m http.server 4173 --directory docs
```

Keep it in a separate agterm session. Request the two pages and core assets:

```sh
curl -fsS -o /dev/null http://127.0.0.1:4173/
curl -fsS -o /dev/null http://127.0.0.1:4173/docs.html
curl -fsS -o /dev/null http://127.0.0.1:4173/styles.css
curl -fsS -o /dev/null http://127.0.0.1:4173/site.js
curl -fsS -o /dev/null http://127.0.0.1:4173/images/jotmd.png
```

Expected: every request exits 0.

- [ ] **Step 3: Re-run the durable link and anchor validation**

Run: `go test ./internal/sitecheck -count=1`

Expected: PASS; the tests make real HTTP requests for both pages, referenced
assets, relative page links, and fragment targets.

- [ ] **Step 4: Run the repository check**

Run: `just check`

Expected: formatting, `go vet ./...`, and `go test ./... -count=1` all pass,
including the static-site integration tests.
all pass.

- [ ] **Step 5: Review the final diff and status**

Run:

```sh
git diff --stat HEAD~2..HEAD
git status --short
```

Expected: the worktree is clean; the two site commits contain `.nojekyll`,
`index.html`, `docs.html`, `styles.css`, `site.js`, and the static-site test.
