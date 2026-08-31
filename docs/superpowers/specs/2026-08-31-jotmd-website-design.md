# JotMD website design

## Goal

Create an English product website for JotMD that can be published from the
repository's `docs/` directory with GitHub Pages. The site must make two ideas
clear immediately:

1. JotMD is a keyboard-first Markdown TUI for the terminal.
2. Its optional `jot-memory` integration gives coding agents durable,
   human-readable memory in the same Markdown vault.

The site has one concise landing page and one separate, detailed documentation
page.

## Audience

The primary audience is developers who keep notes as plain files and prefer
terminal workflows. A secondary audience is users of Codex, Claude Code, or
OpenCode who want durable project memory without a proprietary data store.

## Delivery

The site is dependency-free static HTML, CSS, and minimal JavaScript:

- `docs/index.html` — product landing page;
- `docs/docs.html` — detailed documentation;
- `docs/styles.css` — shared responsive visual system;
- `docs/site.js` — copy-to-clipboard enhancement for command blocks;
- `docs/.nojekyll` — serve static assets without Jekyll processing.

GitHub Pages will publish the `docs/` directory from the default branch. All
internal links and asset paths must be relative so the site works below a
repository subpath such as `/jot/` and under a local HTTP server.

No package manager, framework, generated build output, external font, analytics,
or new dependency is required.

## Visual direction

The approved direction is the terminal-first hybrid, developed as a distinct
JotMD identity rather than a copy of another product site.

- Near-black background and terminal-like surfaces.
- Large, condensed-feeling system sans-serif headlines paired with system
  monospace for navigation, commands, labels, and key bindings.
- Existing mint (`#99d58f`) and coral (`#ef8d8a`) accents.
- Hard borders and offset shadows instead of soft cards and generic gradients.
- Asymmetric layouts and oversized type provide the playful poster quality.
- The existing `docs/images/jotmd.png` screenshot is the main product proof and
  participates in the composition instead of sitting in a generic device card.
- Motion is limited to small entrance or hover transitions and disabled through
  `prefers-reduced-motion`.

Revdiff's public site is only a reference for concise terminal-product
communication and documentation discoverability. JotMD keeps its own layout,
palette, typography, geometry, copy, and content hierarchy.

## Shared navigation

Both pages share a compact header with:

- the `jotmd_` text mark linked to the landing page;
- `Features` and `Agent memory` anchors on the landing page;
- `Docs` linked to `docs.html`;
- `GitHub` linked to the repository.

On small screens the links wrap or reduce to the essential `Docs` and `GitHub`
links without requiring a JavaScript menu.

## Landing page

### Hero

The first viewport states that JotMD is a terminal TUI, not a web note-taking
application.

- Eyebrow: `Markdown notes, zero ceremony.`
- Headline: `Your notes. Right where you work.`
- Description includes the phrase `keyboard-first Markdown TUI for your
  terminal` and promises browsing, search, and preview of plain files.
- Primary action: installation command
  `brew install gonfff/tap/jotmd` with a copy control.
- Secondary action: `Read the docs`.
- The real JotMD screenshot is visible in or immediately after the first
  viewport, depending on screen width.

### Product capabilities

A compact feature section presents product behavior through terminal-flavored
labels and key glyphs rather than generic marketing icons:

- browse directories and Markdown files;
- search note names and contents;
- render live, wrapping Markdown previews;
- work through configurable keyboard bindings;
- manage notes without changing their plain-file format;
- choose from bundled or custom themes.

### Terminal workflow

Show a short three-step sequence:

1. Point JotMD at a vault.
2. Navigate, search, preview, and edit from the TUI.
3. Keep ordinary Markdown files that remain usable by other tools.

### Agent memory

Agent memory is a prominent secondary feature, not the main product identity.
The section uses the visual flow `recall → work → capture` and explains:

- `jot-memory` is optional;
- Codex, Claude Code, and OpenCode can recall and maintain useful context;
- memory remains ordinary Markdown under `agent-memory/` in the selected vault;
- the TUI hides that directory by default and `a` toggles its visibility;
- a link opens the detailed Agent memory section on `docs.html`.

### Installation CTA and footer

Repeat the Homebrew command, link to the source repository and license, and
provide a direct documentation link. Do not add forms, newsletters, social
feeds, testimonials, pricing, or speculative roadmap content.

## Documentation page

The documentation page is a detailed reference, not a second landing page. It
uses a two-column desktop layout with a compact, tree-inspired sticky sidebar
and readable main content. On small screens the sidebar becomes a wrapped table
of contents above the content.

Sections:

1. **Getting started**
   - requirements;
   - Homebrew installation;
   - source build;
   - first launch and explicit `--notes-dir` setup.
2. **Using the TUI**
   - layout and focus;
   - browsing, previewing, and editing;
   - creating, moving, copying, trashing, and deleting notes;
   - search, table of contents, raw view, command palette, and reload behavior.
3. **Essential keys**
   - current default key bindings in a scannable table;
   - link or command for dumping all effective bindings.
4. **Configuration and themes**
   - precedence: CLI flags, environment, TOML, defaults;
   - configuration locations and validation;
   - theme export/list commands and custom themes;
   - editor selection.
5. **CLI**
   - `search`, `get`, `write`, and `delete` forms;
   - JSON output;
   - safe revision-based updates and deletes;
   - warning that deletion is permanent.
6. **Agent memory**
   - purpose and storage layout;
   - prerequisite and initial vault selection;
   - installation, update, and removal for Codex, Claude Code, and OpenCode;
   - verification workflow;
   - visibility through `a` and `show_agent_memory`;
   - reserved `agent-memory/` namespace warning.
7. **Development and links**
   - source, issue tracker, license, and concise build/test commands.

Current `README.md`, `docs/jot-memory.md`, runtime help, configuration templates,
and keymap definitions are the source of truth. The website must not invent
commands or options. Detailed content may be duplicated into static HTML because
GitHub Pages cannot render the existing repository Markdown into the designed
page without adding a build system; changes should remain easy to compare with
those sources.

## Interaction and accessibility

- Semantic landmarks, headings, lists, tables, buttons, and links.
- A visible skip link and visible `:focus-visible` states.
- Sufficient contrast for text, controls, and code.
- Copy controls announce success in their own accessible label/text and never
  replace the selectable command text.
- Horizontal overflow is contained within code blocks and tables, not the page.
- Layout remains usable at narrow mobile widths and large text sizes.
- The site remains fully readable when JavaScript is unavailable.

## Validation

- Serve `docs/` through a local HTTP server and request both pages and referenced
  assets over HTTP.
- Check internal anchors and relative links for both the repository subpath and
  local-server cases.
- Exercise copy controls in a browser when browser control is available.
- Inspect desktop and mobile layouts when browser control is available.
- Run `git diff --check` and the repository's `just check` before handoff.

## Compatibility and risks

- Adding `index.html`, `docs.html`, shared assets, and `.nojekyll` under `docs/`
  does not change the Go application or public CLI API.
- Publishing requires the repository's GitHub Pages source to be configured as
  the default branch's `/docs` directory. Current remote Pages settings could not
  be inspected because the local GitHub CLI credentials are expired.
- Product documentation can drift from runtime behavior. Keeping sections close
  to the existing README structure and checking commands against source reduces
  this risk without introducing a documentation generator.

## Out of scope

- Automatic deployment workflow or custom domain configuration.
- Search within the documentation page.
- Theme switching on the website.
- Generated illustrations, videos, telemetry, analytics, or external services.
- Changes to JotMD application behavior, CLI commands, themes, or agent-memory
  behavior.
