// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgentCoreImageBuildArgsTagCurrentContext(t *testing.T) {
	args := strings.Join(AgentCoreImageBuildArgs(DefaultAgentCoreImage), " ")
	if args != "build -t "+DefaultAgentCoreImage+" ." {
		t.Fatalf("build args = %q", args)
	}
}

type imageRunCall struct {
	dir  string
	env  []string
	name string
	args []string
}

type writtenFile struct {
	name string
	data []byte
}

// fakeImageBuilder returns an imageBuilder whose boundaries record calls and
// whose command runner can be scripted to fail a specific command.
func fakeImageBuilder(failCmd string) (*[]imageRunCall, *[]writtenFile, imageBuilder) {
	var runs []imageRunCall
	var written []writtenFile
	b := imageBuilder{
		run: func(dir string, env []string, name string, args ...string) error {
			runs = append(runs, imageRunCall{dir: dir, env: env, name: name, args: args})
			if name == failCmd {
				return fmt.Errorf("%s exploded", name)
			}
			return nil
		},
		writeFile: func(name string, data []byte, _ os.FileMode) error {
			// Capture the contents here: the context dir is removed when build
			// returns, so the file cannot be read afterward.
			written = append(written, writtenFile{name: name, data: append([]byte(nil), data...)})
			return nil
		},
		copyTree: func(src, dst string) error {
			runs = append(runs, imageRunCall{name: "copyTree", args: []string{src, dst}})
			return nil
		},
	}
	return &runs, &written, b
}

func TestBuildAgentCoreImageInvocationContract(t *testing.T) {
	runs, written, b := fakeImageBuilder("")
	if err := b.build("/core", "declarative-agents/agent-core:local"); err != nil {
		t.Fatalf("build: %v", err)
	}

	// Expected order: go build, copyTree, docker build (Dockerfile written between).
	if len(*runs) != 3 {
		t.Fatalf("recorded %d run/copy calls, want 3: %#v", len(*runs), *runs)
	}
	build := (*runs)[0]
	if build.name != "go" || build.dir != "/core" {
		t.Fatalf("build call = %+v, want go in /core", build)
	}
	buildArgs := strings.Join(build.args, " ")
	if !strings.Contains(buildArgs, "-tags production") || !strings.Contains(buildArgs, "-trimpath") ||
		!strings.HasSuffix(buildArgs, "./cmd/agent") {
		t.Fatalf("go build args = %q, missing production/trimpath/target", buildArgs)
	}
	if !containsEnv(build.env, "CGO_ENABLED=0") || !containsEnv(build.env, "GOOS=linux") {
		t.Fatalf("go build env = %v, want CGO_ENABLED=0 and GOOS=linux", build.env)
	}

	cp := (*runs)[1]
	if cp.name != "copyTree" || cp.args[0] != filepath.Join("/core", "tools") {
		t.Fatalf("copyTree call = %+v, want /core/tools source", cp)
	}
	if !strings.HasSuffix(cp.args[1], filepath.Join("tools")) {
		t.Fatalf("copyTree dst = %q, want a tools subdir of the context", cp.args[1])
	}

	docker := (*runs)[2]
	if docker.name != "docker" ||
		strings.Join(docker.args, " ") != "build -t declarative-agents/agent-core:local ." {
		t.Fatalf("docker call = %+v, want docker build in the context", docker)
	}

	if len(*written) != 1 || filepath.Base((*written)[0].name) != "Dockerfile" {
		t.Fatalf("wrote %v, want exactly one Dockerfile", *written)
	}
	data := (*written)[0].data
	for _, want := range []string{
		"FROM alpine:3.22", "COPY agent /usr/local/bin/agent",
		"COPY tools /opt/agent-core/tools", "ENV AGENT_CORE_HOME=/opt/agent-core",
		"ENTRYPOINT [\"agent\"]",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("Dockerfile missing %q:\n%s", want, data)
		}
	}
	// The docker context directory is cleaned up on return.
	if _, err := os.Stat(docker.dir); !os.IsNotExist(err) {
		t.Errorf("context dir %s not removed: %v", docker.dir, err)
	}
}

func TestBuildAgentCoreImageBuildFailure(t *testing.T) {
	_, _, b := fakeImageBuilder("go")
	err := b.build("/core", "img")
	if err == nil || !strings.Contains(err.Error(), "build linux agent") {
		t.Fatalf("err = %v, want wrapped build failure", err)
	}
}

func TestBuildAgentCoreImageDockerFailure(t *testing.T) {
	_, _, b := fakeImageBuilder("docker")
	err := b.build("/core", "img")
	if err == nil || !strings.Contains(err.Error(), "docker build img") {
		t.Fatalf("err = %v, want wrapped docker failure", err)
	}
}

func TestBuildAgentCoreImagePropagatesCopyTreeError(t *testing.T) {
	copyErr := errors.New("tools tree missing")
	b := imageBuilder{
		run:       func(string, []string, string, ...string) error { return nil },
		writeFile: os.WriteFile,
		copyTree:  func(string, string) error { return copyErr },
	}
	if err := b.build("/core", "img"); !errors.Is(err, copyErr) {
		t.Fatalf("err = %v, want copy-tree error", err)
	}
}

func TestBuildAgentCoreImagePropagatesDockerfileWriteError(t *testing.T) {
	writeErr := errors.New("read-only context")
	b := imageBuilder{
		run:       func(string, []string, string, ...string) error { return nil },
		writeFile: func(string, []byte, os.FileMode) error { return writeErr },
		copyTree:  func(string, string) error { return nil },
	}
	err := b.build("/core", "img")
	if err == nil || !strings.Contains(err.Error(), "write agent-core image Dockerfile") ||
		!errors.Is(err, writeErr) {
		t.Fatalf("err = %v, want wrapped Dockerfile write error", err)
	}
}

func TestCopyTreeContentsCopiesRegularFilesOnly(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "builtin", "otlp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "builtin", "otlp", "all.yaml"), []byte("tools: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A symlink and (best-effort) a FIFO are nonregular entries that must be
	// skipped rather than copied.
	if runtime.GOOS != "windows" {
		if err := os.Symlink("all.yaml", filepath.Join(src, "builtin", "otlp", "link.yaml")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}
	dst := filepath.Join(t.TempDir(), "tools")
	if err := copyTreeContents(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "builtin", "otlp", "all.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "tools: []\n" {
		t.Fatalf("copied content = %q", data)
	}
	if runtime.GOOS != "windows" {
		if _, err := os.Lstat(filepath.Join(dst, "builtin", "otlp", "link.yaml")); !os.IsNotExist(err) {
			t.Fatalf("nonregular symlink was copied: %v", err)
		}
	}
}

func TestCopyTreeContentsManyFiles(t *testing.T) {
	src := t.TempDir()
	const count = 500 // well past a typical descriptor soft limit if leaked.
	for i := 0; i < count; i++ {
		name := filepath.Join(src, fmt.Sprintf("f%03d.txt", i))
		if err := os.WriteFile(name, []byte(fmt.Sprintf("line-%d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dst := filepath.Join(t.TempDir(), "out")
	if err := copyTreeContents(src, dst); err != nil {
		t.Fatalf("copyTreeContents: %v", err)
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != count {
		t.Fatalf("copied %d files, want %d", len(entries), count)
	}
}

func TestCopyRegularFileErrors(t *testing.T) {
	t.Run("open error on missing source", func(t *testing.T) {
		err := copyRegularFile(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "dst"))
		if err == nil {
			t.Fatal("expected open error")
		}
	})

	t.Run("write path blocked by a file parent", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src")
		if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
		// dstParent is a regular file, so MkdirAll of its "subdir" must fail.
		dstParent := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(dstParent, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := copyRegularFile(src, filepath.Join(dstParent, "child", "dst"))
		if err == nil {
			t.Fatal("expected mkdir/create error under a file parent")
		}
	})

	t.Run("read error on an unreadable source", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root bypasses file permissions")
		}
		src := filepath.Join(t.TempDir(), "src")
		if err := os.WriteFile(src, []byte("data"), 0o000); err != nil {
			t.Fatal(err)
		}
		err := copyRegularFile(src, filepath.Join(t.TempDir(), "dst"))
		if err == nil {
			t.Fatal("expected open error on unreadable source")
		}
	})

	t.Run("copies content and reports success", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src")
		if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(t.TempDir(), "nested", "dst")
		if err := copyRegularFile(src, dst); err != nil {
			t.Fatalf("copyRegularFile: %v", err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "payload" {
			t.Fatalf("copied = %q, want payload", got)
		}
	})
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}
