package theme

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gonfff/jotmd/themes"
)

var errInvalidName = errors.New("invalid theme name")

func Names() []string {
	matches, _ := fs.Glob(themes.FS, "gallery/*.toml")
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(match, "gallery/"), ".toml"))
	}
	sort.Strings(names)
	return names
}

func Builtin(name string) (Theme, error) {
	if err := validateName(name); err != nil {
		return Theme{}, err
	}
	contents, err := fs.ReadFile(themes.FS, "gallery/"+name+".toml")
	if errors.Is(err, fs.ErrNotExist) {
		return Theme{}, fmt.Errorf("unknown theme %q", name)
	}
	if err != nil {
		return Theme{}, fmt.Errorf("read bundled theme %q: %w", name, err)
	}
	return decode(contents, name)
}

func EnsureBundled(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create themes directory: %w", err)
	}
	for _, themeName := range Names() {
		name := themeName + ".toml"
		contents, err := fs.ReadFile(themes.FS, "gallery/"+name)
		if err != nil {
			return fmt.Errorf("read bundled theme %q: %w", name, err)
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("create bundled theme %q: %w", name, err)
		}
		if _, err := out.Write(contents); err != nil {
			_ = out.Close()
			return fmt.Errorf("write bundled theme %q: %w", name, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close bundled theme %q: %w", name, err)
		}
	}
	return nil
}

func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read themes directory: %w", err)
	}
	names := make(map[string]struct{})
	for _, name := range Names() {
		names[name] = struct{}{}
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".toml") {
			names[strings.TrimSuffix(entry.Name(), ".toml")] = struct{}{}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func Load(dir, name string) (Theme, error) {
	if err := validateName(name); err != nil {
		return Theme{}, err
	}
	contents, err := os.ReadFile(filepath.Join(dir, name+".toml"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Builtin(name)
		}
		return Theme{}, fmt.Errorf("read theme %q: %w", name, err)
	}
	return decode(contents, name)
}

func Dump(w io.Writer, theme Theme) error {
	if err := validate(theme); err != nil {
		return err
	}
	if err := toml.NewEncoder(w).Encode(theme); err != nil {
		return fmt.Errorf("dump theme: %w", err)
	}
	return nil
}

func Apply(th Theme, overrides map[string]string, noColor bool) (Theme, error) {
	for role, value := range overrides {
		if !chroma.ParseColour(value).IsSet() {
			return Theme{}, fmt.Errorf("invalid color %q for %s", value, role)
		}
		if err := setPaletteColor(&th.Palette, role, value); err != nil {
			return Theme{}, err
		}
	}
	if noColor {
		th.Palette = Palette{}
	}
	return th, nil
}

func setPaletteColor(palette *Palette, role, value string) error {
	switch role {
	case "background":
		palette.Background = value
	case "foreground":
		palette.Foreground = value
	case "muted":
		palette.Muted = value
	case "border":
		palette.Border = value
	case "border_focus":
		palette.BorderFocus = value
	case "selection_background":
		palette.SelectionBackground = value
	case "selection_foreground":
		palette.SelectionForeground = value
	case "accent":
		palette.Accent = value
	case "directory":
		palette.Directory = value
	case "heading":
		palette.Heading = value
	case "link":
		palette.Link = value
	case "code":
		palette.Code = value
	case "status":
		palette.Status = value
	default:
		return fmt.Errorf("unknown color %q", role)
	}
	return nil
}

func decode(contents []byte, name string) (Theme, error) {
	var theme Theme
	metadata, err := toml.Decode(string(contents), &theme)
	if err != nil {
		return Theme{}, fmt.Errorf("decode theme %q: %w", name, err)
	}
	for _, unknown := range metadata.Undecoded() {
		switch unknown.String() {
		case "palette.error", "palette.warning", "palette.success":
			continue
		default:
			return Theme{}, fmt.Errorf("unknown theme field %q", unknown)
		}
	}
	if err := validate(theme); err != nil {
		return Theme{}, fmt.Errorf("invalid theme %q: %w", name, err)
	}
	return theme, nil
}

func validateName(name string) error {
	if name == "" || filepath.Base(name) != name || strings.TrimSuffix(name, ".toml") != name {
		return fmt.Errorf("%w %q", errInvalidName, name)
	}
	return nil
}

func validate(theme Theme) error {
	fields := []struct {
		name  string
		value string
	}{
		{"name", theme.Name},
		{"syntax_style", theme.SyntaxStyle},
		{"palette.foreground", theme.Palette.Foreground},
		{"palette.muted", theme.Palette.Muted},
		{"palette.border", theme.Palette.Border},
		{"palette.border_focus", theme.Palette.BorderFocus},
		{"palette.selection_background", theme.Palette.SelectionBackground},
		{"palette.selection_foreground", theme.Palette.SelectionForeground},
		{"palette.accent", theme.Palette.Accent},
		{"palette.directory", theme.Palette.Directory},
		{"palette.heading", theme.Palette.Heading},
		{"palette.link", theme.Palette.Link},
		{"palette.code", theme.Palette.Code},
		{"palette.status", theme.Palette.Status},
	}
	for _, field := range fields {
		if field.value == "" {
			return fmt.Errorf("%s must not be empty", field.name)
		}
	}
	if styles.Registry[theme.SyntaxStyle] == nil {
		return fmt.Errorf("syntax_style %q is not supported", theme.SyntaxStyle)
	}
	if theme.Palette.Background != "" && !chroma.ParseColour(theme.Palette.Background).IsSet() {
		return fmt.Errorf("palette.background has invalid color %q", theme.Palette.Background)
	}
	for _, field := range fields[2:] {
		if !chroma.ParseColour(field.value).IsSet() {
			return fmt.Errorf("%s has invalid color %q", field.name, field.value)
		}
	}
	return nil
}
