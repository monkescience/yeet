// Package yeet exposes the repository-root assets that ship inside the binary.
//
// The schema lives at the repository root because its published URL is written
// into every generated config, and go:embed cannot reach outside its own
// package directory.
package yeet

import _ "embed"

//go:embed yeet.schema.json
var ConfigSchema []byte
