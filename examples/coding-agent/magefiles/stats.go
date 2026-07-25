// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "fmt"

// Stats reports no additional reusable agent implementations. The persistent
// profiles under agents/serving are application composition around the
// planner, executor, and critic already counted by agent-profiles.
func Stats() {
	fmt.Println(`{}`)
}
