//go:build tools
// +build tools

package starlark

// Fix tool versions in go.mod
import (
	_ "golang.org/x/tools/cmd/stringer"
)
