# Минимальный agent CLI для jotmd — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Добавить к jotmd минимальный agent API из команд search, get, write и revision-safe rm, сохранив TUI без аргументов и Markdown-файлы как единственный source of truth.

**Architecture:** internal/app получает небольшой dispatcher для agent subcommands и JSON/error contract, а filesystem и concurrency guarantees остаются в internal/notes. Existing Store.Read и Store.SearchContent переиспользуются; новые Store.Write и Store.TrashRevision выполняют revision check внутри staged mutation, а не в CLI.

**Tech Stack:** Go 1.26, стандартная библиотека, existing internal/config и internal/notes, Darwin/Linux openat/rename primitives через уже подключённый golang.org/x/sys/unix.

**Spec:** docs/superpowers/specs/2026-08-24-minimal-agent-cli-design.md

## Global Constraints

- cmd/jotmd/main.go и имя бинарника не менять.
- jotmd без subcommand по-прежнему запускает TUI и existing first-run flow.
- Agent subcommands никогда не prompt'ят и не создают config/root implicitly.
- Не добавлять dependencies и CLI framework.
- Paths остаются notes-root-relative; absolute paths, .. и symlink traversal запрещены.
- Markdown-файлы остаются единственным source of truth; не создавать sidecars, index или database.
- Existing config precedence остаётся defaults → config → env → --notes-dir.
- Не менять существующие TUI/config/theme flags и config check semantics.
- Не менять TUI callers internal/notes; добавлять новые APIs или внутренние helpers.
- Не переформатировать целые файлы и не затрагивать concurrent changes.
- Все fields output DTO задавать явно во всех composite literals.
- Platform-specific код оставлять в existing Darwin/Linux files с build constraints.
- Каждый task начинается с failing tests и заканчивается package-level verification.

---

## Карта файлов

### Создать

- internal/app/agent_cli.go — parse/dispatch, config/store loading, plain/JSON output, error mapping.
- internal/app/agent_cli_test.go — parser и end-to-end app.Run tests.
- internal/notes/revision.go — единое вычисление и validation revision.
- internal/notes/write.go — staged create и conditional atomic replace.
- internal/notes/write_test.go — create/update/conflict/concurrency/cleanup tests.
- internal/notes/trash_revision.go — staged revision validation перед platform trash.

### Изменить

- internal/app/run.go — один early dispatcher call перед legacy FlagSet.
- internal/app/run_test.go — compatibility tests.
- internal/notes/read.go и read_test.go — общий revision helper.
- internal/notes/search.go и search_test.go — точный SearchStats.Truncated.
- internal/notes/staging.go и identity.go — stage в подготовленную directory и recovery sentinel.
- internal/notes/trash_darwin.go, trash_linux.go, trash_unsupported.go — revision-safe trash без fallback.
- platform trash tests — revision match/mismatch.
- README.md — четыре agent commands и safe workflow.

---

### Task 1: Agent argv parser без изменения legacy TUI

**Files:**
- Create: internal/app/agent_cli.go
- Create: internal/app/agent_cli_test.go

**Interfaces:**
- Consumes: Run(ctx, args, stdin, stdout, stderr, releaseVersion...) int.
- Produces:

~~~go
type agentInvocation struct {
	command    string
	notesDir   *string
	jsonOutput bool
	help       bool
	limit      int
	ifRevision *string
	operands   []string
}

func parseAgentInvocation([]string) (agentInvocation, bool, error)
~~~

bool означает, что argv принадлежит search|get|write|rm; false оставляет argv existing legacy parser.

- [ ] **Step 1: Написать failing parser table test**

~~~go
func TestParseAgentInvocationAcceptsFlagsAroundOperands(t *testing.T) {
	tests := []struct {
		args        []string
		command     string
		operands    []string
		notesDir    string
		jsonOutput  bool
		limit       int
		ifRevision string
	}{
		{args: []string{"get", "docs/a.md", "--json"}, command: "get", operands: []string{"docs/a.md"}, jsonOutput: true, limit: 20},
		{args: []string{"--notes-dir", "./vault", "search", "auth", "--limit", "7", "--json"}, command: "search", operands: []string{"auth"}, notesDir: "./vault", jsonOutput: true, limit: 7},
		{args: []string{"write", "docs/a.md", "--if-revision", "sha256:abc"}, command: "write", operands: []string{"docs/a.md"}, ifRevision: "sha256:abc", limit: 20},
		{args: []string{"rm", "--if-revision=sha256:def", "docs/a.md"}, command: "rm", operands: []string{"docs/a.md"}, ifRevision: "sha256:def", limit: 20},
	}
	for _, tt := range tests {
		got, handled, err := parseAgentInvocation(tt.args)
		if err != nil || !handled {
			t.Fatalf("parseAgentInvocation(%q) = (%#v, %v, %v)", tt.args, got, handled, err)
		}
		gotNotesDir, gotRevision := "", ""
		if got.notesDir != nil {
			gotNotesDir = *got.notesDir
		}
		if got.ifRevision != nil {
			gotRevision = *got.ifRevision
		}
		if got.command != tt.command || !slices.Equal(got.operands, tt.operands) ||
			gotNotesDir != tt.notesDir || got.jsonOutput != tt.jsonOutput ||
			got.limit != tt.limit || gotRevision != tt.ifRevision {
			t.Errorf("parseAgentInvocation(%q) = %#v", tt.args, got)
		}
	}
}
~~~

Добавить cases:

- empty args, --help, --theme nord, config check → handled=false;
- unknown flag, missing option value, duplicate command → parse error;
- invalid/non-positive/greater-than-200 limit → parse error;
- --limit вне search и --if-revision вне write|rm → parse error;
- rm PATH без revision → parse error;
- -- прекращает option parsing.

- [ ] **Step 2: Запустить test и подтвердить RED**

~~~bash
go test ./internal/app -run TestParseAgentInvocation -count=1
~~~

Expected: compile failure, функции ещё не определены.

- [ ] **Step 3: Реализовать single-pass parser**

Распознавать только --notes-dir, --json, --limit, --if-revision, --help/-h и --. Поддержать формы --name value и --name=value. Первый positional token — command; остальные — operands. Default limit задать явно как 20. После scan проверить command-specific flags и ровно один QUERY/PATH.

Legacy TUI flags не переносить. Если первый positional token не agent command, вернуть handled=false.

- [ ] **Step 4: Оставить parser изолированным от Run**

На этом task не менять run.go и не перехватывать production invocations. Wiring
выполняется в Task 3 одновременно с первыми рабочими командами, поэтому каждый
commit сохраняет используемый CLI работоспособным.

- [ ] **Step 5: Зафиксировать legacy detection tests**

Проверить непосредственно parseAgentInvocation, что nil args, --help,
--version, --theme nord и config check возвращают handled=false без error.

- [ ] **Step 6: GREEN**

~~~bash
go test ./internal/app -count=1
~~~

- [ ] **Step 7: Commit только task files**

~~~bash
git add internal/app/agent_cli.go internal/app/agent_cli_test.go
git commit -m "feat: dispatch minimal agent CLI commands"
~~~

---

### Task 2: Общая revision primitive и точный search truncation

**Files:**
- Create: internal/notes/revision.go
- Modify: internal/notes/read.go:3-10,88-94
- Modify: internal/notes/read_test.go:15-55
- Modify: internal/notes/search.go:31-34,62-114
- Modify: internal/notes/search_test.go:110-139

**Interfaces:**

~~~go
var ErrInvalidRevision = errors.New("invalid revision")
var ErrRevisionConflict = errors.New("revision conflict")

type RevisionConflictError struct {
	Expected Revision
	Actual   Revision
}

func (*RevisionConflictError) Unwrap() error
func ParseRevision(string) (Revision, error)
func revisionBytes([]byte) Revision
func revisionFile(*os.File) (Revision, error)

type SearchStats struct {
	FilesScanned int
	FilesSkipped int
	Truncated    bool
}
~~~

- [ ] **Step 1: Написать failing revision tests**

~~~go
func TestParseRevisionAcceptsOnlySHA256Token(t *testing.T) {
	valid := "sha256:" + strings.Repeat("a", 64)
	got, err := ParseRevision(valid)
	if err != nil || got != Revision(valid) {
		t.Fatalf("ParseRevision(valid) = (%q, %v)", got, err)
	}
	for _, value := range []string{"", "abc", "sha256:abc", "sha256:" + strings.Repeat("z", 64)} {
		if _, err := ParseRevision(value); !errors.Is(err, ErrInvalidRevision) {
			t.Errorf("ParseRevision(%q) error = %v", value, err)
		}
	}
}
~~~

Добавить assertion, что revisionBytes(content) совпадает с Store.Read revision.

- [ ] **Step 2: Написать failing truncation test**

~~~go
func TestSearchContentReportsOnlyRealTruncation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		lines     int
		truncated bool
	}{
		{name: "exact limit", lines: 2, truncated: false},
		{name: "more than limit", lines: 3, truncated: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			content := strings.Repeat("needle\n", tt.lines)
			writeTree(t, root, nil, []treeFile{{path: "note.md", content: content, mode: 0o600}})
			store, err := NewStore(root, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			matches, stats, err := store.SearchContent(context.Background(), "needle", 1024, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 2 || stats.Truncated != tt.truncated {
				t.Fatalf("SearchContent() = %d matches, %#v", len(matches), stats)
			}
		})
	}
}
~~~

Existing request limit=1000 продолжает возвращать максимум 200.

- [ ] **Step 3: RED**

~~~bash
go test ./internal/notes -run 'TestParseRevision|TestSearchContentReportsOnlyRealTruncation' -count=1
~~~

- [ ] **Step 4: Реализовать revision.go**

ParseRevision принимает только sha256: + 64 hex digits через hex.DecodeString и
возвращает canonical lowercase token через hex.EncodeToString, поэтому uppercase
input сравнивается корректно. revisionBytes использует sha256.Sum256.
revisionFile использует sha256.New + io.Copy на уже безопасно открытом file и
возвращает wrapped read errors. RevisionConflictError.Unwrap возвращает
ErrRevisionConflict.

В Store.Read заменить inline hash на revisionBytes(content).

- [ ] **Step 5: Сделать truncation точным**

SearchContent внутренне допускает limit+1 matches. Extra match выставляет Truncated=true и не входит в returned slice. Если scan закончился ровно на limit, Truncated=false. Ordering, Unicode folding и public cap 200 не менять.

- [ ] **Step 6: GREEN**

~~~bash
go test ./internal/notes -run 'TestParseRevision|TestRead|TestSearch' -count=1
~~~

- [ ] **Step 7: Commit**

~~~bash
git add internal/notes/revision.go internal/notes/read.go internal/notes/read_test.go internal/notes/search.go internal/notes/search_test.go
git commit -m "feat: expose stable note revisions"
~~~

---

### Task 3: search, get, JSON и stable CLI errors

**Files:**
- Modify: internal/app/agent_cli.go
- Modify: internal/app/agent_cli_test.go
- Modify: internal/app/run.go:39-65
- Modify: internal/app/run_test.go:42-78

**Interfaces:**

~~~go
func loadAgentStore(*string) (*notes.Store, error)
func runAgentCLI(context.Context, agentInvocation, io.Reader, io.Writer, io.Writer) int
func runSearch(context.Context, *notes.Store, agentInvocation, io.Writer) error
func runGet(context.Context, *notes.Store, agentInvocation, io.Writer) error
func writeAgentError(io.Writer, bool, error) int
func parseAgentNotePath(string) (notes.RelPath, error)

type searchJSON struct {
	Query     string             `json:"query"`
	Results   []searchResultJSON `json:"results"`
	Truncated bool               `json:"truncated"`
	Stats     searchStatsJSON    `json:"stats"`
}

type searchResultJSON struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type searchStatsJSON struct {
	FilesScanned int `json:"files_scanned"`
	FilesSkipped int `json:"files_skipped"`
}

type getJSON struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Content  string `json:"content"`
}

type errorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}
~~~

- [ ] **Step 1: Написать failing search Run tests**

~~~go
func TestRunAgentSearchJSON(t *testing.T) {
	root := t.TempDir()
	writeAgentNote(t, root, "projects/foo.md", "refresh token redis pitfall\n")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"--notes-dir", root, "search", "refresh token redis", "--json",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(search) = (%d, %q), stderr = %q", code, stdout.String(), stderr.String())
	}
	var got searchJSON
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Query != "refresh token redis" || got.Truncated || len(got.Results) != 1 {
		t.Fatalf("search JSON = %#v", got)
	}
	result := got.Results[0]
	if result.Path != "projects/foo.md" || result.Line != 1 ||
		result.Snippet != "refresh token redis pitfall" {
		t.Errorf("search result = %#v", result)
	}
}
~~~

Добавить no matches → results:[]; --limit 1 over two matches → truncated true; plain format path:line:  snippet; missing root → exit 3.

- [ ] **Step 2: Написать failing get tests**

Проверить:

- plain exact bytes без appended newline;
- JSON содержит только path, revision, content;
- missing note → exit 3;
- directory, non-.md, absolute и .. path → exit 2;
- invalid UTF-8 работает в plain mode, но JSON возвращает invalid_encoding exit 2.

- [ ] **Step 3: Написать failing error envelope tests**

Unknown agent flag с --json: stdout empty, stderr декодируется как
error.code=invalid_input. Parser определяет наличие --json до возврата parse
error, даже если unknown flag расположен раньше. Human mode пишет
jotmd: <code>: <message> newline.

- [ ] **Step 4: RED**

~~~bash
go test ./internal/app -run 'TestRunAgent(Search|Get)|TestAgentJSONError' -count=1
~~~

- [ ] **Step 5: Wire dispatcher и сохранить legacy behavior**

В начале Run вызвать parseAgentInvocation. handled=false продолжает existing
FlagSet без изменений. handled=true с parse error вызывает agent error writer;
valid invocation передаётся в runAgentCLI. Help обрабатывается до config/root
loading.

Добавить compatibility tests для --help, --version, config check и existing TUI
validation с прежними exit/stdout/stderr contracts.

- [ ] **Step 6: Реализовать data-only config pipeline**

loadAgentStore выполняет:

~~~text
config.DefaultPath → config.Load → config.ApplyEnv → optional NotesDir Merge
→ config.Finalize → notes.NewStore → SetReadMaxBytes(cfg.Preview.MaxBytes)
~~~

Не вызывать setupFirstRun, editor.Resolve, theme/keymap loading или TUI setup.

- [ ] **Step 7: Реализовать note-path boundary и outputs**

parseAgentNotePath отклоняет empty path, absolute path, components . и ..,
backslash/NUL и filename без case-insensitive .md suffix. Store повторно
проверяет path и symlinks как domain safety boundary.

search вызывает SearchContent(ctx, query, 8<<20, limit), переносит Truncated и создаёт non-nil results slice. Plain format: path:line:  snippet newline.

get вызывает Store.Read; plain mode пишет Document.Content напрямую. Перед JSON проверить utf8.Valid. Encoder использует SetEscapeHTML(false) и всегда добавляет один newline.

- [ ] **Step 8: Реализовать error mapping**

Порядок errors.Is:

~~~text
ErrInvalidRevision или usage error              → exit 2
fs.ErrNotExist                                  → exit 3
ErrRevisionConflict или ErrExists               → exit 4
fs.ErrPermission или ErrTrashPermissionDenied   → exit 5
остальное                                       → exit 1
~~~

JSON error идёт только в stderr; plain error включает stable code.

- [ ] **Step 9: GREEN**

~~~bash
go test ./internal/app -count=1
~~~

- [ ] **Step 10: Commit**

~~~bash
git add internal/app/agent_cli.go internal/app/agent_cli_test.go internal/app/run.go internal/app/run_test.go
git commit -m "feat: add agent search and get commands"
~~~

---

### Task 4: Atomic create и conditional Store.Write

**Files:**
- Create: internal/notes/write.go
- Create: internal/notes/write_test.go
- Modify: internal/notes/staging.go:26-45,73-88
- Modify: internal/notes/identity.go:13-16

**Interfaces:**

~~~go
var ErrRecoveryRequired = errors.New("filesystem recovery required")

type WriteResult struct {
	Path     RelPath
	Revision Revision
	Created  bool
}

func (s *Store) Write(context.Context, RelPath, []byte, *Revision) (WriteResult, error)
func (staged *stagedEntry) stage(FileIdentity) error
~~~

- [ ] **Step 1: Написать failing write tests**

Создать tests:

~~~go
func TestWriteCreatesMissingNoteWithoutOverwriting(t *testing.T)
func TestWriteUpdatesOnlyMatchingRevision(t *testing.T)
func TestWriteRejectsMissingTargetWithRevision(t *testing.T)
func TestWriteAcceptsEmptyContent(t *testing.T)
func TestWriteRejectsDirectoryMissingParentAndNonMarkdownPath(t *testing.T)
~~~

Использовать existing writeTree, identityFor и assertNoStagingDirectories. Проверять exact content, revision, Created, сохранение old content на conflict и отсутствие staging.

- [ ] **Step 2: Написать concurrent writer test**

~~~go
func TestWriteAllowsOnlyOneWriterForTheSameRevision(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "old", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, content := range [][]byte{[]byte("writer-a"), []byte("writer-b")} {
		content := content
		go func() {
			<-start
			_, err := store.Write(context.Background(), "note.md", content, &document.Revision)
			results <- err
		}()
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("Write() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	content, err := os.ReadFile(filepath.Join(root, "note.md"))
	if err != nil || string(content) != "writer-a" && string(content) != "writer-b" {
		t.Fatalf("final content = %q, error = %v", content, err)
	}
	assertNoStagingDirectories(t, root)
}
~~~

- [ ] **Step 3: RED**

~~~bash
go test ./internal/notes -run TestWrite -count=1
~~~

- [ ] **Step 4: Refactor existing staging**

Из stageCheckedEntry выделить staged.stage(expected). Method выполняет existing renameNoReplace parent/name → staged/name и identity verification. Existing Rename/Move/Delete/Trash callers продолжают использовать stageCheckedEntry без semantic changes.

Если restore невозможен, error должен оборачивать ErrRecoveryRequired и сохранять existing absolute recovery path.

- [ ] **Step 5: Реализовать create-only flow**

1. Validate .md path и открыть parent.
2. Создать sibling staging directory.
3. Создать replacement, записать content, Sync, Close.
4. renameNoReplace replacement → parent/name.
5. EEXIST → ErrExists и полный cleanup.
6. Вернуть WriteResult с Created=true.

- [ ] **Step 6: Реализовать conditional update flow**

1. Полностью подготовить replacement в sibling staging.
2. lstat current target; missing → RevisionConflictError с empty Actual.
3. Reject directory, symlink и non-regular target.
4. staged.stage(current identity) переносит target в staging и служит linearization point.
5. Вычислить actual revision staged old file.
6. Mismatch → удалить replacement, restore old, вернуть RevisionConflictError.
7. Match → renameNoReplace replacement → parent/name.
8. Удалить staged old и staging directory.
9. Cleanup failure после visible commit → ErrRecoveryRequired.
10. Вернуть WriteResult с Created=false.

Каждый early return учитывает ctx.Err и не оставляет target частично записанным.

- [ ] **Step 7: GREEN mutation tests**

~~~bash
go test ./internal/notes -run 'TestWrite|TestStage|TestRename|TestCopy|TestMove|TestDelete' -count=1
~~~

- [ ] **Step 8: Commit**

~~~bash
git add internal/notes/write.go internal/notes/write_test.go internal/notes/staging.go internal/notes/identity.go
git commit -m "feat: add revision-safe note writes"
~~~

---

### Task 5: CLI write

**Files:**
- Modify: internal/app/agent_cli.go
- Modify: internal/app/agent_cli_test.go

**Interfaces:**

~~~go
func runWrite(context.Context, *notes.Store, agentInvocation, io.Reader, io.Writer) error

type writeJSON struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Created  bool   `json:"created"`
}
~~~

- [ ] **Step 1: Написать failing Run tests**

Проверить create plain/JSON, existing without condition, matching update, mismatch, missing target with condition, empty stdin, malformed revision, failReader без mutation и failWriter после committed mutation.

~~~go
func TestRunAgentWriteCreatesNoteAndReturnsJSON(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{
		"--notes-dir", root, "write", "note.md", "--json",
	}, strings.NewReader("# Note\n"), &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("Run(write) = (%d, %q), stderr = %q", code, stdout.String(), stderr.String())
	}
	var got writeJSON
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != "note.md" || got.Revision == "" || !got.Created {
		t.Fatalf("write JSON = %#v", got)
	}
	content, err := os.ReadFile(filepath.Join(root, "note.md"))
	if err != nil || string(content) != "# Note\n" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}
~~~

- [ ] **Step 2: RED**

~~~bash
go test ./internal/app -run TestRunAgentWrite -count=1
~~~

- [ ] **Step 3: Реализовать input и revision handling**

Прочитать stdin через io.ReadAll. Empty stdin оставить valid. Если flag задан, ParseRevision выполнить до Store.Write и передать pointer; иначе nil.

- [ ] **Step 4: Реализовать output и recovery error mapping**

Plain: новая revision + newline. JSON: path, revision, created с explicit fields. Conflict details: path и expected_revision; actual_revision не публиковать, чтобы agent всегда делал новый get.

Добавить notes.ErrRecoveryRequired → exit 6, code recovery_required в общий
error mapper, поскольку sentinel появляется в Task 4.

- [ ] **Step 5: GREEN**

~~~bash
go test ./internal/app ./internal/notes -count=1
~~~

- [ ] **Step 6: Commit**

~~~bash
git add internal/app/agent_cli.go internal/app/agent_cli_test.go
git commit -m "feat: add conditional agent write command"
~~~

---

### Task 6: Revision-safe system trash и CLI rm

**Files:**
- Create: internal/notes/trash_revision.go
- Modify: internal/notes/trash_darwin.go:18-63
- Modify: internal/notes/trash_linux.go:18-127
- Modify: internal/notes/trash_unsupported.go:3-12
- Modify: internal/notes/trash_darwin_test.go
- Modify: internal/notes/trash_linux_test.go
- Modify: internal/app/agent_cli.go
- Modify: internal/app/agent_cli_test.go

**Interfaces:**

~~~go
func (s *Store) TrashRevision(context.Context, RelPath, Revision) error
func (s *Store) stageRevision(context.Context, RelPath, Revision) (*stagedEntry, FileIdentity, error)
func (s *Store) finishStagedTrash(context.Context, RelPath, FileIdentity, *stagedEntry) error
~~~

trash_revision.go имеет build constraint darwin || linux, потому что использует
stagedEntry. Unsupported method остаётся в trash_unsupported.go.

- [ ] **Step 1: Написать failing Darwin tests**

Через existing runFinderTrash override проверить matching revision, mismatch без Finder invocation, restore source, отсутствие staging и conflict при replacement race.

~~~go
func TestTrashRevisionRejectsMismatchBeforeFinder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "current", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	original := runFinderTrash
	called := false
	runFinderTrash = func(context.Context, string) error { called = true; return nil }
	t.Cleanup(func() { runFinderTrash = original })
	stale := revisionBytes([]byte("stale"))
	err = store.TrashRevision(context.Background(), "note.md", stale)
	if !errors.Is(err, ErrRevisionConflict) || called {
		t.Fatalf("TrashRevision() error = %v, Finder called = %v", err, called)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "note.md"))
	if readErr != nil || string(content) != "current" {
		t.Fatalf("restored content = %q, error = %v", content, readErr)
	}
	assertNoStagingDirectories(t, root)
}
~~~

- [ ] **Step 2: Написать failing Linux tests**

С XDG_DATA_HOME=t.TempDir проверить matching move в Trash/files + .trashinfo, mismatch без trash artifacts и cross-filesystem restore без permanent fallback.

~~~go
func TestTrashRevisionMovesMatchingNoteToXDGTrash(t *testing.T) {
	root := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "current", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.TrashRevision(context.Background(), "note.md", document.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(dataHome, "Trash", "files", "note.md")); err != nil || string(content) != "current" {
		t.Fatalf("trashed content = %q, error = %v", content, err)
	}
}
~~~

- [ ] **Step 3: RED**

~~~bash
go test ./internal/notes -run TestTrashRevision -count=1
~~~

- [ ] **Step 4: Реализовать stageRevision**

1. Validate .md path.
2. Open parent и lstat child.
3. Reject directory/symlink/non-regular.
4. stageCheckedEntry по current identity.
5. Вычислить revision staged file.
6. Mismatch → restore + RevisionConflictError.
7. Match → вернуть staged и identity.
8. Любой error закрывает descriptors и сохраняет source.

- [ ] **Step 5: Refactor platform finish**

Darwin existing finishStagedTrash привести к общей method signature. Linux выделить metadata/final rename tail в такой же method. Existing Trash продолжает identity-based flow. TrashRevision использует stageRevision. Unsupported platform возвращает existing unsupported error. Delete никогда не вызывается как fallback.

- [ ] **Step 6: Написать failing app rm tests**

Проверить missing/malformed revision, mismatch exit 4, directory/non-Markdown exit 2 и plain empty stdout. Success JSON тестировать через runRemove seam:

~~~go
func runRemove(
	ctx context.Context,
	invocation agentInvocation,
	output io.Writer,
	trash func(context.Context, notes.RelPath, notes.Revision) error,
) error
~~~

Production передаёт store.TrashRevision; test передаёт deterministic function без Finder.

- [ ] **Step 7: Реализовать rm output**

~~~go
type removeJSON struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	Action   string `json:"action"`
}
~~~

Единственное v1 action — trashed. Все fields задать явно.

- [ ] **Step 8: GREEN**

~~~bash
go test ./internal/notes -run TestTrash -count=1
go test ./internal/app -run 'TestRunAgentRemove|TestParseAgentInvocation' -count=1
go test ./internal/app ./internal/notes -count=1
~~~

- [ ] **Step 9: Commit только собственные hunks**

Сначала просмотреть existing dirty platform files:

~~~bash
git diff -- internal/notes/trash_darwin.go internal/notes/trash_darwin_test.go internal/notes/trash_linux.go internal/notes/trash_linux_test.go
~~~

Затем использовать git add -p для overlapping files и не stage'ить unrelated changes.

~~~bash
git add -p internal/notes/trash_darwin.go internal/notes/trash_darwin_test.go internal/notes/trash_linux.go internal/notes/trash_linux_test.go
git add internal/notes/trash_revision.go internal/notes/trash_unsupported.go internal/app/agent_cli.go internal/app/agent_cli_test.go
git commit -m "feat: add revision-safe agent remove command"
~~~

---

### Task 7: Help, документация и backward compatibility

**Files:**
- Modify: internal/app/agent_cli.go
- Modify: internal/app/agent_cli_test.go
- Modify: internal/app/run_test.go
- Modify: README.md
- Verify: cmd/jotmd/main.go unchanged

**Interfaces:**
- Consumes: полный command/error contract Tasks 1–6.
- Produces: root/command help, README workflow и verified compatibility.

- [ ] **Step 1: Написать failing help tests**

Проверить jotmd --help и help для search/get/write/rm: exit 0, stdout help, stderr empty, config/root не читаются. Root help не рекламирует create, list, context, permanent delete или fuzzy search.

~~~go
func TestRunAgentCommandHelpDoesNotLoadConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "missing"))
	for _, command := range []string{"search", "get", "write", "rm"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{command, "--help"}, strings.NewReader(""), &stdout, &stderr)
			if code != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
				t.Fatalf("Run(%s --help) = (%d, %q, %q)", command, code, stdout.String(), stderr.String())
			}
		})
	}
}
~~~

- [ ] **Step 2: RED**

~~~bash
go test ./internal/app -run 'TestRun.*Help' -count=1
~~~

- [ ] **Step 3: Реализовать concise help**

Help явно фиксирует:

- search — contiguous case-insensitive substring, limit 1..200;
- get — plain content или JSON content+revision;
- write — create missing, existing требует if-revision;
- rm — одна note, revision required, system trash, no permanent fallback;
- flags можно ставить после operands, -- завершает options.

- [ ] **Step 4: Обновить README минимальным hunk**

Добавить раздел Agent CLI с command tree, search → get → write workflow и revision-safe rm. Не переписывать install/TUI/theme sections и не форматировать README целиком.

- [ ] **Step 5: Focused checks**

~~~bash
go test ./internal/app ./internal/notes -count=1
git diff --check
~~~

- [ ] **Step 6: Full project check**

~~~bash
just check
~~~

Expected: just format check, go vet и go test ./... проходят.

- [ ] **Step 7: Built binary compatibility**

~~~bash
just build
./bin/jotmd --help
./bin/jotmd --version
./bin/jotmd get --help
./bin/jotmd write --help
./bin/jotmd rm --help
git diff --exit-code -- cmd/jotmd/main.go
~~~

Bare TUI не запускать в non-interactive verification.

- [ ] **Step 8: Scope review**

~~~bash
git status --short
git diff --stat
git diff --check
~~~

Убедиться, что нет dependencies, database/index/sidecars, filesystem wrapper commands или unrelated formatting.

- [ ] **Step 9: Commit docs/help отдельно**

README и overlapping files stage'ить через git add -p.

~~~bash
git add -p README.md internal/app/agent_cli.go internal/app/agent_cli_test.go internal/app/run_test.go
git add docs/superpowers/specs/2026-08-24-minimal-agent-cli-design.md docs/superpowers/plans/2026-08-24-minimal-agent-cli.md
git commit -m "docs: document minimal agent CLI"
~~~

---

## Compatibility risks

- Legacy parser останавливается на первом positional token. Agent parser должен остаться narrow dispatcher; полная замена legacy parser расширит scope.
- README.md, internal/notes/trash_darwin.go и related tests уже modified в shared worktree. Implementation patch'ит только нужные hunks.
- Revision CAS защищает cooperating/path-based writers. Процесс с уже открытым descriptor может менять staged inode вне protocol; database из-за этого не добавляется.
- Linux trash может быть на другом filesystem; macOS Finder может потребовать Automation permission. Permanent fallback запрещён.
- JSON get требует valid UTF-8; plain get остаётся byte-preserving.

## Финальная матрица проверок

| Contract | Tests | Verification |
|---|---|---|
| Legacy TUI/config dispatch | internal/app/run_test.go | go test ./internal/app |
| Flags around operands | internal/app/agent_cli_test.go | parser table |
| Search semantics/truncation | notes search + app JSON tests | notes + app |
| Revision format | internal/notes/read_test.go | read/revision |
| Atomic create/CAS write | internal/notes/write_test.go | concurrent writer |
| Revision-safe trash | Darwin/Linux trash tests | platform notes |
| JSON/error/exit codes | internal/app/agent_cli_test.go | public Run |
| Full compatibility | all packages | just check |
