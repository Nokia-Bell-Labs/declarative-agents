// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/magefile/mage/mg"
)

// doltComposeFile is the persistent local Dolt sql-server definition consumed by
// the dolt: mage targets.
const doltComposeFile = "docker-compose.dolt.yml"

// Dolt manages the persistent local Dolt sql-server used by lifecycle
// checkpoints (--dolt-dsn) and the gated Dolt integration tests. Storage lives
// in a named volume, so Down keeps the data and only Reset discards it.
type Dolt mg.Namespace

// Up starts the Dolt sql-server in the background. Data persists in the dolt-data
// volume across container removal.
func (Dolt) Up() error {
	return doltCompose("up", "-d")
}

// Down stops and removes the container, keeping the dolt-data volume so the
// persisted checkpoints survive.
func (Dolt) Down() error {
	return doltCompose("down")
}

// Reset stops the container and deletes the dolt-data volume, discarding all
// persisted Dolt data.
func (Dolt) Reset() error {
	return doltCompose("down", "-v")
}

// Status shows the Dolt service state and the persistent volume.
func (Dolt) Status() error {
	if err := doltCompose("ps"); err != nil {
		return err
	}
	cmd := exec.Command(dockerEngine, "volume", "ls", "--filter", "name=dolt-data")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// doltCompose runs a docker compose subcommand against the Dolt compose file.
func doltCompose(args ...string) error {
	full := append([]string{"compose", "-f", doltComposeFile}, args...)
	fmt.Printf("+ %s %s\n", dockerEngine, strings.Join(full, " "))
	cmd := exec.Command(dockerEngine, full...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
