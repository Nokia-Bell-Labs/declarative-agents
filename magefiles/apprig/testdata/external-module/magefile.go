// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

//go:build mage

package main

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/magefile/mage/mg"
)

// App is a downstream module's complete lifecycle surface. Every method is a
// thin call to the versioned package; the module owns configuration only.
type App mg.Namespace

func (App) Up() error { return downstreamRunner().Up() }

func (App) Status() error {
	report, err := downstreamRunner().Status()
	if err != nil {
		return err
	}
	data, err := json.Marshal(report)
	if err == nil {
		fmt.Println(string(data))
	}
	return err
}

func (App) Down() error {
	report, err := downstreamRunner().Down()
	if err != nil {
		return err
	}
	fmt.Printf("%s retained; query=%s\n", report.Bucket.Detail, report.QueryEndpoint.State)
	return nil
}

func (App) Diagnose() error { return downstreamRunner().Diagnose() }
func (App) Purge() error    { return downstreamRunner().PurgeData() }

func downstreamRunner() apprig.Runner {
	return apprig.Runner{
		ManifestPath: "application.yaml",
		Binding: apprig.PlatformBinding{
			Cluster: "da-platform", ApplicationRoot: ".",
			ChartPath: "helm", ValuesPath: "helm/values.yaml", Timeout: "5m",
		},
		CatalogRoot: "../catalog",
	}
}
