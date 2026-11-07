package content

import "embed"

//go:embed all:maps all:scenarios all:units
var Files embed.FS
