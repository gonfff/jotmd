package theme

type Palette struct {
	Background          string `toml:"background,omitempty"`
	Foreground          string `toml:"foreground"`
	Muted               string `toml:"muted"`
	Border              string `toml:"border"`
	BorderFocus         string `toml:"border_focus"`
	SelectionBackground string `toml:"selection_background"`
	SelectionForeground string `toml:"selection_foreground"`
	Accent              string `toml:"accent"`
	Directory           string `toml:"directory"`
	Heading             string `toml:"heading"`
	Link                string `toml:"link"`
	Code                string `toml:"code"`
	Status              string `toml:"status"`
}

type Theme struct {
	Name        string  `toml:"name"`
	Palette     Palette `toml:"palette"`
	SyntaxStyle string  `toml:"syntax_style"`
}
