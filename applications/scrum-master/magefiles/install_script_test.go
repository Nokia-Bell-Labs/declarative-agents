// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/spectool"
)

// TestInstallScriptInstallsAVerifiedRelease builds the release for this
// machine at a tag in a clone of this repository, as mage publish builds it
// — the agent and the profiles archive — serves it, and proves install.sh,
// run with no Go toolchain on PATH, installs an agent whose version is the
// tag and the profiles at that release, that the init profile runs from
// them with only the agent installed, and that a file whose checksum
// SHA256SUMS does not list is refused (eng03, GH-34).
func TestInstallScriptInstallsAVerifiedRelease(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("install.sh serves Linux and macOS")
	}
	// The test clones the repository holding the application, inside the
	// one that ships agent-core (GH-190) or on its own (GH-2754), and
	// publishes from the application's directory in the clone.
	app := mustAbs(t, "..")
	top, err := exec.Command("git", "-C", app, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(top))
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if a, err := filepath.EvalSymlinks(app); err == nil {
		app = a
	}
	inRepo, err := filepath.Rel(root, app)
	if err != nil {
		t.Fatal(err)
	}
	head, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	const tag = "v0.20990101.1"
	clone := filepath.Join(t.TempDir(), "monorepo")
	for _, args := range [][]string{
		// --no-tags: a HEAD that is itself a release, as main is right
		// after mage tag, must not lend the clone its tag (GH-41).
		{"clone", "-q", "--no-checkout", "--no-tags", root, clone},
		{"-C", clone, "checkout", "-q", strings.TrimSpace(string(head))},
		{"-C", clone, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "tag", "-a", tag, "-m", "Release " + tag},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	dist := t.TempDir()
	appRoot := filepath.Join(clone, inRepo)
	// The parent checkout an application outside the repository names is
	// one git ignores, so the clone takes it from the original.
	if b, err := os.ReadFile(filepath.Join(app, spectool.CheckoutsFile)); err == nil {
		if err := os.WriteFile(filepath.Join(appRoot, spectool.CheckoutsFile), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := release.Publisher{Root: appRoot, Git: release.ExecGit(appRoot), Run: release.ExecCommand}
	assets, err := p.Build(dist, []release.Platform{{OS: runtime.GOOS, Arch: runtime.GOARCH}})
	if err != nil {
		t.Fatal(err)
	}
	served := map[string]string{}
	var srv *httptest.Server
	var links []string
	for _, a := range assets {
		served["/download/"+tag+"/"+a.Name] = a.Path
		// A GitLab project keeps a release's files as uploads, which only
		// the API serves to a token (GH-31, GH-2767).
		served["/api/v4/projects/7/uploads/5ec7e7/"+a.Name] = a.Path
		links = append(links, fmt.Sprintf(`{"id":%d,"name":%q,"url":"%%s/-/project/7/uploads/5ec7e7/%s","link_type":"other"}`, len(links)+1, a.Name, a.Name))
	}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		api := strings.HasPrefix(path, "/api/v4/")
		switch {
		case api && r.Header.Get("PRIVATE-TOKEN") != "t0ken":
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
		case path == "/api/v4/projects/group%2Fdeclarative-agents/releases/"+tag:
			var b strings.Builder
			for i, l := range links {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(fmt.Sprintf(l, srv.URL))
			}
			fmt.Fprintf(w, `{"tag_name":%q,"assets":{"links":[%s]}}`, tag, b.String())
		case strings.Contains(path, "/-/releases/") && strings.Contains(path, "/downloads/"):
			// The downloads route redirects to the upload's web URL ...
			http.Redirect(w, r, srv.URL+"/-/project/7/uploads/5ec7e7/"+filepath.Base(path), http.StatusFound)
		case strings.HasPrefix(path, "/-/project/"):
			// ... which serves a session, never a token.
			http.Redirect(w, r, srv.URL+"/users/sign_in", http.StatusFound)
		case path == "/users/sign_in":
			http.Error(w, "sign in", http.StatusForbidden)
		default:
			p, ok := served[path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, p)
		}
	}))
	defer srv.Close()

	install := func(prefix, home string) (string, error) {
		cmd := exec.Command("sh", filepath.Join(app, "install.sh"), tag)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "AGENT_PREFIX=" + prefix, "SCRUM_MASTER_HOME=" + home,
			"SCRUM_MASTER_RELEASE_URL=" + srv.URL, "SCRUM_MASTER_RELEASE_FORGE=github"}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	prefix, home := filepath.Join(t.TempDir(), "bin"), filepath.Join(t.TempDir(), "scrum-master")
	out, err := install(prefix, home)
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	version, err := exec.Command(filepath.Join(prefix, "agent"), "--version").Output()
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(version)), tag) {
		t.Fatalf("the installed agent reports %q, want %s: %v", version, tag, err)
	}
	if b, err := os.ReadFile(filepath.Join(home, "release")); err != nil || strings.TrimSpace(string(b)) != tag {
		t.Fatalf("the installed profiles are at %q, want %s: %v", b, tag, err)
	}
	if !strings.Contains(out, "agent --profile") {
		t.Errorf("install.sh did not name the next step:\n%s", out)
	}

	// With only the agent installed, the init profile lays a repository down,
	// installing the skills of the one agent the repository already uses
	// (GH-49).
	repo := filepath.Join(t.TempDir(), "system")
	if out, err := exec.Command("git", "init", "-q", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := exec.Command(filepath.Join(prefix, "agent"), "--profile", filepath.Join(home, "agents", "init", "profile.yaml"),
		"--core-root", filepath.Join(home, "agent-core"), "--directory", repo, "--output", t.TempDir())
	run.Dir = repo
	run.Env = []string{"PATH=" + prefix + ":/usr/bin:/bin", "HOME=" + t.TempDir(), "SCRUM_MASTER_HOME=" + home}
	if out, err := run.CombinedOutput(); err != nil || !strings.Contains(string(out), "final machine state: Done") {
		t.Fatalf("the init profile from the installed release: %v\n%s", err, out)
	}
	if b, err := os.ReadFile(filepath.Join(repo, ".scrum-master", "manifest.json")); err != nil || !strings.Contains(string(b), tag) {
		t.Fatalf("the init run vendored no profiles at %s: %v\n%s", tag, err, b)
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "skills", "pop", "SKILL.md")); err != nil {
		t.Fatalf("the init run installed no claude skill in a repository using claude: %v", err)
	}
	for _, other := range []string{".cursor", ".github"} {
		if _, err := os.Stat(filepath.Join(repo, other)); !os.IsNotExist(err) {
			t.Fatalf("the init run wrote %s in a repository using only claude", other)
		}
	}

	// The same release from a GitLab project, as an internal fork publishes
	// it: installed with the project's token, refused without (GH-31).
	gitlab := func(token string) (string, error) {
		cmd := exec.Command("sh", filepath.Join(app, "install.sh"), tag)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "AGENT_PREFIX=" + filepath.Join(t.TempDir(), "bin"),
			"SCRUM_MASTER_HOME=" + filepath.Join(t.TempDir(), "home"), "SCRUM_MASTER_RELEASE_FORGE=gitlab",
			"SCRUM_MASTER_RELEASE_URL=" + srv.URL + "/group/declarative-agents", "GITLAB_TOKEN=" + token}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := gitlab("t0ken"); err != nil || !strings.Contains(out, "installed the scrum-master profiles "+tag) {
		t.Fatalf("install.sh from a GitLab project: %v\n%s", err, out)
	}
	if out, err := gitlab(""); err == nil {
		t.Fatalf("a private GitLab project's release installed with no token:\n%s", out)
	}

	tampered := filepath.Join(t.TempDir(), "tampered")
	os.WriteFile(tampered, []byte("not the agent"), 0o755)
	served["/download/"+tag+"/"+assets[0].Name] = tampered
	other := filepath.Join(t.TempDir(), "bin")
	if out, err := install(other, filepath.Join(t.TempDir(), "home")); err == nil || !strings.Contains(out, "refusing") {
		t.Fatalf("a tampered agent was installed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(other, "agent")); !os.IsNotExist(err) {
		t.Fatalf("a refused agent was written: %v", err)
	}

	// A release missing a file it should carry is refused naming the file
	// and the release, not with curl's bare status (GH-2751).
	served["/download/"+tag+"/"+assets[0].Name] = assets[0].Path
	delete(served, "/download/"+tag+"/"+release.ProfilesArchive)
	if out, err := install(filepath.Join(t.TempDir(), "bin"), filepath.Join(t.TempDir(), "home")); err == nil ||
		!strings.Contains(out, "could not download "+release.ProfilesArchive+" of release "+tag) {
		t.Fatalf("a release missing its profiles archive: %v\n%s", err, out)
	}
}
