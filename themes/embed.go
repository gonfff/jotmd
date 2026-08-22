// Package themes embeds the theme gallery shipped with JotMD.
package themes

import "embed"

//go:embed gallery/*.toml
var FS embed.FS
