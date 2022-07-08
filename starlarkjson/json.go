// Copyright 2020 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package starlarkjson is an alias for github.com/canonical/starlark/lib/json to provide
// backwards compatibility
//
// Deprecated: use github.com/canonical/starlark/lib/json instead
package starlarkjson // import "github.com/canonical/starlark/stalarkjson"

import (
	"github.com/canonical/starlark/lib/json"
)

// Module is an alias of json.Module for backwards import compatibility
var Module = json.Module
