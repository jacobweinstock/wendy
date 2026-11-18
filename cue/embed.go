// Package cue embeds wendy's CUE package so the binary carries its own schema.
package cue

import "embed"

// FS holds the wendy CUE package under wendy/.
//
//go:embed wendy/*.cue
var FS embed.FS

// Starter is the commented config wendy writes on first launch.
//
//go:embed starter.cue.tmpl
var Starter []byte
