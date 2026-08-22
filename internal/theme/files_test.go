package theme

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureBundledCreatesJotMDTheme(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")
	if err := EnsureBundled(dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "jotmd.toml"))
	if err != nil {
		t.Fatalf("bundled jotmd theme was not created: %v", err)
	}
	if !strings.HasPrefix(string(contents), "# JotMD default theme\n") {
		t.Fatalf("bundled jotmd theme lost its source comments:\n%s", contents)
	}
}

func TestEnsureBundledDoesNotOverwriteExistingTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jotmd.toml")
	const custom = "custom theme\n"
	if err := os.WriteFile(path, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureBundled(dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != custom {
		t.Errorf("EnsureBundled() overwrote %q with %q", custom, contents)
	}
}

func TestLoadRejectsMalformedAndIncompleteTheme(t *testing.T) {
	dir := t.TempDir()
	bundled := builtinThemeTOML(t)
	writeTheme(t, dir, "broken", strings.Replace(bundled, "foreground = \"#E9E2D5\"\n", "", 1))
	if _, err := Load(dir, "broken"); err == nil || !strings.Contains(err.Error(), "palette.foreground") {
		t.Errorf("Load() error = %v, want missing palette.foreground error", err)
	}
	writeTheme(t, dir, "malformed", "[palette\n")
	if _, err := Load(dir, "malformed"); err == nil {
		t.Error("Load() malformed theme error = nil")
	}
}

func TestLoadRejectsInvalidPaletteColorAndSyntaxStyle(t *testing.T) {
	dir := t.TempDir()
	bundled := builtinThemeTOML(t)
	writeTheme(t, dir, "bad-color", strings.Replace(bundled, "[palette]\n", "[palette]\nbackground = \"not-a-color\"\n", 1))
	if _, err := Load(dir, "bad-color"); err == nil || !strings.Contains(err.Error(), "palette.background") {
		t.Errorf("Load() error = %v, want invalid palette.background error", err)
	}
	writeTheme(t, dir, "bad-style", strings.Replace(bundled, "syntax_style = \"rose-pine\"", "syntax_style = \"not-a-style\"", 1))
	if _, err := Load(dir, "bad-style"); err == nil || !strings.Contains(err.Error(), "syntax_style") {
		t.Errorf("Load() error = %v, want invalid syntax_style error", err)
	}
}

func TestLoadPrefersThemeFileOverSameNamedBuiltin(t *testing.T) {
	dir := t.TempDir()
	contents := strings.Replace(builtinThemeTOML(t), "[palette]\n", "[palette]\nbackground = \"#000000\"\n", 1)
	writeTheme(t, dir, "jotmd", contents)

	got, err := Load(dir, "jotmd")
	if err != nil {
		t.Fatal(err)
	}
	if got.Palette.Background != "#000000" {
		t.Errorf("Load() Background = %q, want value from theme file", got.Palette.Background)
	}
}

func TestListSortsThemeNames(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "zebra", "")
	writeTheme(t, dir, "alpha", "")
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := append(append([]string{"alpha"}, Names()...), "zebra"); !reflect.DeepEqual(got, want) {
		t.Errorf("List() = %#v, want %#v", got, want)
	}
}

func TestDumpRoundTripsThemeTOML(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureBundled(dir); err != nil {
		t.Fatal(err)
	}
	want, err := Load(dir, "jotmd")
	if err != nil {
		t.Fatal(err)
	}
	var dumped bytes.Buffer
	if err := Dump(&dumped, want); err != nil {
		t.Fatal(err)
	}
	writeTheme(t, dir, "roundtrip", dumped.String())
	got, err := Load(dir, "roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Load(Dump(theme)) = %#v, want %#v", got, want)
	}
}

func TestDumpOmitsUnusedPaletteRoles(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Dump(&output, th); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"error =", "warning =", "success ="} {
		if strings.Contains(output.String(), field) {
			t.Errorf("Dump() contains unused palette role %q:\n%s", field, output.String())
		}
	}
}

func TestDumpAcceptsAndOmitsTerminalBackground(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	th.Palette.Background = ""
	var output bytes.Buffer
	if err := Dump(&output, th); err != nil {
		t.Fatalf("Dump() terminal-background theme error = %v", err)
	}
	if strings.Contains(output.String(), "\n  background =") {
		t.Fatalf("Dump() paints an empty background:\n%s", output.String())
	}
}

func TestLoadAcceptsLegacyUnusedPaletteRoles(t *testing.T) {
	dir := t.TempDir()
	contents := builtinThemeTOML(t) + "  error = \"#FF0000\"\n  warning = \"#FFFF00\"\n  success = \"#00FF00\"\n"
	writeTheme(t, dir, "legacy", contents)
	if _, err := Load(dir, "legacy"); err != nil {
		t.Fatalf("Load() legacy theme error = %v", err)
	}
}

func TestLoadRejectsUnsafeThemeNames(t *testing.T) {
	if _, err := Load(t.TempDir(), "../jotmd"); !errors.Is(err, errInvalidName) {
		t.Errorf("Load() error = %v, want invalid name", err)
	}
}

func writeTheme(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func builtinThemeTOML(t *testing.T) string {
	t.Helper()
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Dump(&output, th); err != nil {
		t.Fatal(err)
	}
	return output.String()
}
