// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import "encoding/json"

// catalogUnmarshal keeps the test's config literals as JSON, which YAML
// accepts, without importing a YAML package into the tests.
func catalogUnmarshal(config string, target *map[string]interface{}) error {
	return json.Unmarshal([]byte(config), target)
}
