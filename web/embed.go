// Package web embeds the built front-end into the binary.
package web

import "embed"

// Dist holds the Vite build output.
//
//go:embed all:dist
var Dist embed.FS
