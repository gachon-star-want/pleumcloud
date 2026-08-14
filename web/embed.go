// Package web embeds the built SPA (web/dist) so the pleumcloud binary
// serves the frontend with zero external files. The dist placeholder is
// committed so fresh clones build; `make build` replaces it with the real
// bundle.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
