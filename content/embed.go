package content

import "embed"

//go:embed all:maps all:scenarios all:units all:misc
var Files embed.FS
