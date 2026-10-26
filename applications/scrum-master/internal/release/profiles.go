// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package release

import (
	"archive/tar"
	"compress/gzip"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// ProfilesArchive is the release's profiles and the agent-core declarations
// they read (GH-34): agents/<name>/..., agent-core/tools/..., and release,
// the tag. install.sh unpacks it beside the agent, and the init profile
// runs from it.
const ProfilesArchive = "scrum-master-profiles.tar.gz"

// WriteProfilesArchive writes the archive at path from the profiles under
// agents and the tools tree under tools. Entries are sorted and carry no
// time, so a rebuild at the same tag writes the same bytes.
func WriteProfilesArchive(path, agents, tools, release string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	files := map[string]string{}
	for _, tree := range []struct{ from, to string }{{agents, "agents"}, {tools, "agent-core/tools"}} {
		err := filepath.WalkDir(tree.from, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(tree.from, p)
			if err != nil {
				return err
			}
			files[tree.to+"/"+filepath.ToSlash(rel)] = p
			return nil
		})
		if err != nil {
			return err
		}
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	write := func(name string, body []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tw.Write(body)
		return err
	}
	for _, n := range names {
		b, err := os.ReadFile(files[n])
		if err != nil {
			return err
		}
		if err := write(n, b); err != nil {
			return err
		}
	}
	if err := write("release", []byte(release+"\n")); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}
