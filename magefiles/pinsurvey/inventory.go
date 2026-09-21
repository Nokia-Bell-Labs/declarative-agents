// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pinsurvey

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed pins.yaml
var declaredPinsDocument string

// declaredPin is one entry of the checked-in list: a pin no chart render
// exposes (srd006 R1.2).
type declaredPin struct {
	Location     string `yaml:"location"`
	File         string `yaml:"file"`
	Image        string `yaml:"image"`
	Tag          string `yaml:"tag"`
	Architecture string `yaml:"architecture"`
	Digest       string `yaml:"digest"`
}

// DeclaredPins returns the checked-in list, rejecting an entry that cannot
// say where it lives or what it pins.
func DeclaredPins() ([]Pin, error) {
	var document struct {
		Pins []declaredPin `yaml:"pins"`
	}
	if err := yaml.Unmarshal([]byte(declaredPinsDocument), &document); err != nil {
		return nil, fmt.Errorf("parse declared pins: %w", err)
	}
	pins := make([]Pin, 0, len(document.Pins))
	for index, entry := range document.Pins {
		var missing []string
		for name, value := range map[string]string{
			"location": entry.Location, "file": entry.File,
			"image": entry.Image, "tag": entry.Tag,
		} {
			if strings.TrimSpace(value) == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return nil, fmt.Errorf("declared pin %d lacks %s", index, strings.Join(missing, ", "))
		}
		pins = append(pins, Pin{
			Location:     entry.Location,
			Image:        entry.Image,
			Tag:          entry.Tag,
			Digest:       entry.Digest,
			Architecture: entry.Architecture,
		})
	}
	return pins, nil
}

// DeclaredPinFiles returns each declared entry with the file it names, so a
// test can check the list still describes what is there (srd006 R1.3).
func DeclaredPinFiles() (map[string]string, error) {
	var document struct {
		Pins []declaredPin `yaml:"pins"`
	}
	if err := yaml.Unmarshal([]byte(declaredPinsDocument), &document); err != nil {
		return nil, fmt.Errorf("parse declared pins: %w", err)
	}
	files := map[string]string{}
	for _, entry := range document.Pins {
		files[entry.Location] = entry.File
	}
	return files, nil
}

// ChartPins turns rendered chart image references into pins. A reference
// carrying no digest is still surveyed: currency applies whether or not a
// digest was recorded.
func ChartPins(chart, overlay string, images []string) ([]Pin, error) {
	pins := make([]Pin, 0, len(images))
	for _, image := range images {
		reference, err := ParseReference(image)
		if err != nil {
			return nil, err
		}
		if reference.Tag == "" {
			continue
		}
		pins = append(pins, Pin{
			Location: fmt.Sprintf("applications/%s/helm [%s]", chart, overlay),
			Image:    referenceImage(reference),
			Tag:      reference.Tag,
			Digest:   reference.Digest,
		})
	}
	return pins, nil
}

// referenceImage renders a reference without its tag or digest.
func referenceImage(reference Reference) string {
	if reference.Registry != "" && reference.Registry != defaultRegistry {
		return reference.Registry + "/" + reference.Repository
	}
	return strings.TrimPrefix(reference.Repository, defaultNamespace+"/")
}

// Deduplicate collapses pins that name the same image, tag and architecture,
// keeping the first location. One image renders in several charts and under
// several overlays, and a report that named it once per render would bury
// the distinct pins among repetitions.
func Deduplicate(pins []Pin) []Pin {
	seen := map[string]bool{}
	var unique []Pin
	for _, pin := range pins {
		// Key on the resolved reference rather than the spelling: a chart
		// writes alpine/k8s where the rig writes docker.io/alpine/k8s, and
		// the same pin reported twice reads as two.
		identity := pin.Image
		if reference, err := ParseReference(pin.Image); err == nil {
			identity = reference.Registry + "/" + reference.Repository
		}
		key := strings.Join([]string{identity, pin.Tag, pin.Digest, pin.Architecture}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, pin)
	}
	sort.SliceStable(unique, func(i, j int) bool { return unique[i].Location < unique[j].Location })
	return unique
}

// SkipRepositoryImages drops images this checkout builds or retags. Their
// digests name nothing in a registry and their tags name a release of this
// repository, so a registry has no opinion about either (srd005 R2.2).
func SkipRepositoryImages(pins []Pin, prefixes []string) []Pin {
	var external []Pin
	for _, pin := range pins {
		lowered := strings.ToLower(pin.Image)
		skip := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(lowered, strings.TrimSuffix(prefix, "/")) {
				skip = true
				break
			}
		}
		if !skip {
			external = append(external, pin)
		}
	}
	return external
}

// ReadFile is the file read the declared-pin check uses, named here so the
// package's only filesystem dependency is visible.
func ReadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
