# JotMD Website Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a distinctive English landing page and detailed documentation page for the JotMD terminal TUI from the repository's `docs/` directory.

**Architecture:** Use hand-written static HTML, one shared CSS file, and one progressively enhanced JavaScript file. GitHub Pages serves `docs/` directly; relative URLs keep the site valid beneath the repository path and under a local HTTP server. A small `just site-check` recipe protects the required files, content, and relative-link contract without adding dependencies.

**Tech Stack:** HTML5, CSS, browser JavaScript, POSIX shell assertions in `just`, GitHub Pages

**Spec:** `docs/superpowers/specs/2026-08-31-jotmd-website-design.md`

## Global Constraints

- Keep the site dependency-free: no package manager, framework, generated build output, external font, analytics, or new dependency.
- Publish from `docs/`; all internal URLs and asset paths must be relative and work below `/jot/`.
- Present JotMD first as a keyboard-first Markdown TUI for the terminal.
- Present `jot-memory` as an optional secondary feature for Codex, Claude Code, and OpenCode.
- Use `README.md`, `docs/jot-memory.md`, runtime help, configuration templates, and keymap definitions as the source of truth; do not invent commands.
- Reuse `docs/images/jotmd.png`; do not modify the user's existing README, documentation, or untracked logo concepts.
- Preserve the approved terminal-first visual direction: near-black surfaces, large poster-like system typography, system monospace, mint `#99d58f`, coral `#ef8d8a`, hard borders, and offset shadows.
- Keep the pages useful without JavaScript and honor `prefers-reduced-motion`.

---

### Task 1: Static-site contract and landing page

**Files:**
- Modify: `justfile`
- Create: `docs/index.html`
- Create: `docs/styles.css`
- Create: `docs/site.js`
- Create: `docs/.nojekyll`

**Interfaces:**
- Consumes: `docs/images/jotmd.png`, existing install command and feature descriptions from `README.md`
- Produces: shared classes and tokens in `docs/styles.css`; `data-copy` controls handled by `docs/site.js`; `just site-check`

- [ ] **Step 1: Add the failing landing-page contract**

Append this recipe to `justfile` and call it from `check` before the Go checks:

```just
site-check:
    #!/bin/sh
    set -eu
    test -f docs/index.html
    test -f docs/docs.html
    test -f docs/styles.css
    test -f docs/site.js
    test -f docs/.nojekyll
    rg -q 'keyboard-first Markdown TUI' docs/index.html
    rg -q 'images/jotmd.png' docs/index.html
    rg -q 'href="docs.html"' docs/index.html
    rg -q 'id="agent-memory"' docs/index.html
    rg -q 'id="agent-memory"' docs/docs.html
    ! rg -n '(href|src)="/' docs/index.html docs/docs.html

check: site-check
    @just --fmt --check
    @go vet ./...
    @just test
```

Keep the existing `check` body once; replace its declaration rather than adding
a duplicate recipe.

- [ ] **Step 2: Run the contract and verify it fails**

Run: `just site-check`

Expected: FAIL because `docs/index.html` and `docs/docs.html` do not exist.

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

- [ ] **Step 6: Run the partial contract**

Run: `just site-check`

Expected: FAIL only because `docs/docs.html` has not been created yet. Confirm
the landing-page assertions themselves pass with:

```sh
test -f docs/index.html
rg -q 'keyboard-first Markdown TUI' docs/index.html
rg -q 'images/jotmd.png' docs/index.html
rg -q 'href="docs.html"' docs/index.html
rg -q 'id="agent-memory"' docs/index.html
```

- [ ] **Step 7: Commit the landing page**

```sh
git add justfile docs/.nojekyll docs/index.html docs/styles.css docs/site.js
git commit -m "feat: add JotMD product landing page"
```

### Task 2: Detailed documentation page

**Files:**
- Create: `docs/docs.html`
- Reuse: `docs/styles.css`, `docs/site.js`

**Interfaces:**
- Consumes: shared header, typography, code, table, button, and documentation layout classes from `docs/styles.css`
- Produces: `docs.html#agent-memory` target used by the landing page and complete static product documentation

- [ ] **Step 1: Confirm the documentation contract still fails**

Run: `just site-check`

Expected: FAIL at `test -f docs/docs.html`.

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
    <section id="agent-cli">...</section>
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

- [ ] **Step 4: Add verified Agent CLI documentation**

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

- [ ] **Step 6: Run the site contract**

Run: `just site-check`

Expected: PASS.

- [ ] **Step 7: Commit the documentation page**

```sh
git add docs/docs.html
git commit -m "docs: add detailed JotMD website guide"
```

### Task 3: HTTP and repository validation

**Files:**
- Modify only if validation finds a real defect: `docs/index.html`, `docs/docs.html`, `docs/styles.css`, `docs/site.js`, `justfile`

**Interfaces:**
- Consumes: complete static site from Tasks 1 and 2
- Produces: GitHub Pages-ready files with verified local routing and repository checks

- [ ] **Step 1: Check formatting and the static contract**

Run:

```sh
git diff --check
just site-check
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

- [ ] **Step 3: Validate HTML links and anchors with the standard library**

Run a temporary Python snippet that parses both files with `html.parser`, checks
that every relative file target exists under `docs/`, and checks every local
fragment against an element ID in the target document. Do not add this helper to
the repository; `just site-check` keeps the durable contract while this is the
one-time comprehensive check.

Expected: output `site links: ok`.

- [ ] **Step 4: Run the repository check**

Run: `just check`

Expected: formatting, site contract, `go vet ./...`, and `go test ./... -count=1`
all pass.

- [ ] **Step 5: Review the final diff and status**

Run:

```sh
git diff --stat HEAD~2..HEAD
git status --short
```

Expected: only pre-existing user changes remain unstaged; the two site commits
contain `justfile`, `.nojekyll`, `index.html`, `docs.html`, `styles.css`, and
`site.js`.
