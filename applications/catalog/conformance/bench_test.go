// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBenchConformance launches the bench profile, waits for its generic REST
// health route, starts two overlapping critic runs through the experiments
// capability, lists them, is refused a third at the declared bound, and asks
// the control server for a lifecycle exit so the serve lifecycle stops its
// listeners and reaches Done.
//
// It runs the wrapper an operator ships — agents/bench/profile.yaml — through a
// temp copy, patching the request, control, and monitor listen addresses so
// the test does not collide with a real bench on :8080-:8082, and the launch
// bound from four to two so the bound is reachable with held children. The
// profile's /opt/agent-core tool_config_dir remaps onto the checkout via
// --core-root; nothing else is rebuilt.
//
// The critic child is a recorder that holds for the test's duration, so both
// runs are alive when the listing and the bound are checked; the host's
// shutdown reaps them (srd040 R7.2).
func TestBenchConformance(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	profilePath, addr, controlAddr := stageBench(t, map[string]string{"max_running: 4": "max_running: 2"})
	runDir := t.TempDir()
	child := filepath.Join(runDir, "critic-child")
	capture := filepath.Join(runDir, "child-args.txt")
	writeBenchChildRecorder(t, child)

	server := Serve(t, ServeConfig{
		Profile: profilePath, Directory: ProfilesRoot(),
		Args: []string{"--child-agent-binary", child},
		Env:  []string{"BENCH_CHILD_ARGS=" + capture, "BENCH_CHILD_HOLD=60"},
	})
	server.WaitHealthy("http://"+addr+"/api/v1/health", 15*time.Second)
	requireBenchResponse(t, "http://"+addr+"/", http.StatusOK, "<div id=\"root\"></div>")
	requireBenchResponse(t, "http://"+addr+"/api/v1/sessions", http.StatusOK, `"data":[]`)
	requireBenchResponse(t, "http://"+addr+"/api/v1/configs", http.StatusOK, `"category"`)
	// The host machine is a template instance with no states of its own, so the
	// graph comes from a machine that declares them.
	requireBenchResponse(t, "http://"+addr+"/api/v1/configs/bench/experiments-machine.yaml", http.StatusOK, `"graph"`)
	requireBenchProfiles(t, "http://"+addr+"/api/v1/profiles")
	requireBenchResponse(t, "http://"+addr+"/api/v1/source/agents/bench/machine.yaml", http.StatusOK, `"language":"yaml"`)
	experiments := "http://" + addr + "/api/v1/experiments"
	launch := `{"suite":"suites/basic.yaml","output_dir":"eval-results"}`
	first := requireBenchLaunch(t, experiments, launch, http.StatusAccepted)
	requireBenchChildArgs(t, capture, child)
	second := requireBenchLaunch(t, experiments, launch, http.StatusAccepted)
	if first["service"] == second["service"] {
		t.Fatalf("two launches share handle %v", first["service"])
	}
	requireBenchRunsRunning(t, experiments+"/runs", first["service"], second["service"])
	requireBenchLaunch(t, experiments, launch, http.StatusTooManyRequests)
	requireBenchExit(t, server, controlAddr)
	result := server.WaitExit(15 * time.Second)

	// srd006: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RequireNoErrorSpans(t)

	// srd006: the serve lifecycle words are the visible human-input boundary.
	result.RequireToolSpans(t, "launch_bench_http", "launch_bench_control", "launch_monitor_rest",
		"await_bench_control", "exit_agent", "stop_monitor_rest", "stop_bench_http")
	result.RequireToolSpans(t, "list_evaluation_sessions", "list_resource", "read_resource")
	result.RequireToolSpans(t, "launch_evaluator", "list_experiments")

	// srd006: the host shutdown reaches Done even though request machines also
	// contribute terminal events to the shared trace.
	requireBenchTerminalState(t, result, "Done")
}

func TestBenchProfilesAreCheckoutIndependent(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	profilePath, addr, controlAddr := stageBench(t, nil)
	workspace := t.TempDir()
	server := Serve(t, ServeConfig{Profile: profilePath, Directory: workspace})
	server.WaitHealthy("http://"+addr+"/api/v1/health", 15*time.Second)

	requireBenchProfiles(t, "http://"+addr+"/api/v1/profiles")

	requireBenchExit(t, server, controlAddr)
	result := server.WaitExit(15 * time.Second)
	result.RequireExit(t, 0)
	requireBenchTerminalState(t, result, "Done")
}

// stageBench copies the shipped bench profile with its request, control, and
// monitor listeners moved to free ports, so parallel tests and a real bench on
// :8080-:8082 do not collide, plus any extra patches the caller names.
func stageBench(t *testing.T, extra map[string]string) (profilePath, addr, controlAddr string) {
	t.Helper()
	addr, controlAddr = FreeAddr(t), FreeAddr(t)
	patches := map[string]string{
		"address: 127.0.0.1:8080": "address: " + addr,
		"address: 127.0.0.1:8081": "address: " + controlAddr,
		"127.0.0.1:8082":          FreeAddr(t),
	}
	for match, replacement := range extra {
		patches[match] = replacement
	}
	profilePath = CopyShippedProfile(t, filepath.Join("agents", "bench", "profile.yaml"), patches)
	return profilePath, addr, controlAddr
}

// requireBenchExit asks the bench control server for a lifecycle exit.
func requireBenchExit(t *testing.T, server *Server, controlAddr string) {
	t.Helper()
	endpoint := "http://" + controlAddr + "/api/lifecycle/exit"
	if status := server.Post(endpoint, `{"reason":"conformance","status":"success"}`); status != http.StatusAccepted {
		t.Fatalf("POST %s status = %d, want %d", endpoint, status, http.StatusAccepted)
	}
}

// requireBenchLaunch posts one experiment launch and returns its decoded body.
func requireBenchLaunch(t *testing.T, endpoint, body string, want int) map[string]interface{} {
	t.Helper()
	response, err := http.Post(endpoint, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s: %v", endpoint, err)
	}
	if response.StatusCode != want {
		t.Fatalf("POST %s = %d %s, want %d", endpoint, response.StatusCode, raw, want)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode POST %s body %s: %v", endpoint, raw, err)
	}
	if want == http.StatusAccepted {
		if service, _ := payload["service"].(string); service == "" {
			t.Fatalf("POST %s body %s has no service handle", endpoint, raw)
		}
	}
	return payload
}

// requireBenchRunsRunning reads the experiments listing and requires every
// named run to be present and running.
func requireBenchRunsRunning(t *testing.T, endpoint string, services ...interface{}) {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	var payload struct {
		Data []struct {
			Service string `json:"service"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode GET %s: %v", endpoint, err)
	}
	status := map[string]string{}
	for _, run := range payload.Data {
		status[run.Service] = run.Status
	}
	for _, service := range services {
		if got := status[fmt.Sprint(service)]; got != "running" {
			t.Errorf("run %v status = %q in %+v, want running", service, got, payload.Data)
		}
	}
}

func requireBenchProfiles(t *testing.T, endpoint string) {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer func() { _ = response.Body.Close() }()
	var payload struct {
		Data []struct {
			Name         string `json:"name"`
			StrictFormat bool   `json:"strictFormat"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode GET %s: %v", endpoint, err)
	}
	want := map[string]bool{
		"deepseek": true, "default": false, "gemma": true,
		"glm": false, "granite": false, "laguna": false,
		"mistral": true, "nemotron": false, "qwen": true,
	}
	if response.StatusCode != http.StatusOK || len(payload.Data) != len(want) {
		t.Fatalf("GET %s = %d with %d profiles, want 200 with %d", endpoint, response.StatusCode, len(payload.Data), len(want))
	}
	for _, profile := range payload.Data {
		strict, ok := want[profile.Name]
		if !ok {
			t.Errorf("unexpected bench profile %q", profile.Name)
			continue
		}
		if profile.StrictFormat != strict {
			t.Errorf("profile %q strictFormat = %t, want %t", profile.Name, profile.StrictFormat, strict)
		}
		delete(want, profile.Name)
	}
	if len(want) != 0 {
		t.Errorf("missing bench profiles: %v", want)
	}
}

const benchChildRecorderScript = `#!/bin/sh
set -eu
capture=${BENCH_CHILD_ARGS:?}
tmp="${capture}.$$"
cleanup() {
	rm -f "$tmp"
}
trap cleanup EXIT HUP INT TERM
{
	printf 'pid=%s\000' "$$"
	printf 'executable=%s\000' "$0"
	for arg do
		printf 'arg=%s\000' "$arg"
	done
} > "$tmp"
mv -f "$tmp" "$capture"
trap - EXIT
if [ -n "${BENCH_CHILD_HOLD:-}" ]; then
	exec sleep "$BENCH_CHILD_HOLD"
fi
`

type benchChildRecord struct {
	PID        int
	Executable string
	Args       []string
}

func writeBenchChildRecorder(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(benchChildRecorderScript), 0o755); err != nil {
		t.Fatalf("write critic child recorder: %v", err)
	}
}

func parseBenchChildRecord(data []byte) (benchChildRecord, error) {
	var record benchChildRecord
	for _, field := range bytes.Split(data, []byte{0}) {
		if len(field) == 0 {
			continue
		}
		key, value, ok := bytes.Cut(field, []byte("="))
		if !ok {
			return record, fmt.Errorf("field %q has no separator", field)
		}
		switch string(key) {
		case "pid":
			pid, err := strconv.Atoi(string(value))
			if err != nil || pid <= 0 {
				return record, fmt.Errorf("invalid pid %q", value)
			}
			record.PID = pid
		case "executable":
			record.Executable = string(value)
		case "arg":
			record.Args = append(record.Args, string(value))
		default:
			return record, fmt.Errorf("unknown field %q", key)
		}
	}
	if record.PID == 0 || record.Executable == "" {
		return record, fmt.Errorf("incomplete identity: pid=%d executable=%q",
			record.PID, record.Executable)
	}
	return record, nil
}

func validateBenchChildRecord(record benchChildRecord, expectedExecutable string) error {
	if record.Executable != expectedExecutable {
		return fmt.Errorf("pid=%d executable=%q argv=%q: want executable %q",
			record.PID, record.Executable, record.Args, expectedExecutable)
	}
	for _, expected := range []struct {
		flag, value string
	}{
		{"--profile", "agents/critic/profile.yaml"},
		{"--request", "suites/basic.yaml"},
		{"--output", "eval-results"},
	} {
		found := false
		for index := 0; index+1 < len(record.Args); index++ {
			if record.Args[index] == expected.flag &&
				record.Args[index+1] == expected.value {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf(
				"pid=%d executable=%q argv=%q: missing adjacent %q %q",
				record.PID, record.Executable, record.Args,
				expected.flag, expected.value)
		}
	}
	return nil
}

func requireBenchChildArgs(t *testing.T, path, expectedExecutable string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			record, parseErr := parseBenchChildRecord(data)
			if parseErr != nil {
				t.Fatalf("critic child evidence %s is malformed: %v", path, parseErr)
			}
			if validateErr := validateBenchChildRecord(
				record, expectedExecutable); validateErr != nil {
				t.Fatalf("critic child evidence %s is invalid: %v", path, validateErr)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("critic child did not record args at %s", path)
}

func TestBenchChildRecorderPublishesCompleteConcurrentRecords(t *testing.T) {
	runDir := t.TempDir()
	child := filepath.Join(runDir, "critic-child")
	capture := filepath.Join(runDir, "child-args.bin")
	writeBenchChildRecorder(t, child)

	stopReader := make(chan struct{})
	readerDone := make(chan struct{})
	recordErr := make(chan error, 1)
	var reportOnce sync.Once
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			data, err := os.ReadFile(capture)
			if os.IsNotExist(err) {
				continue
			}
			if err == nil {
				record, parseErr := parseBenchChildRecord(data)
				if parseErr == nil {
					parseErr = validateBenchChildRecord(record, child)
				}
				if parseErr != nil {
					reportOnce.Do(func() { recordErr <- parseErr })
				}
			}
		}
	}()

	var writers sync.WaitGroup
	for range 20 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			command := exec.Command(child,
				"--profile", "agents/critic/profile.yaml",
				"--core-root", "/core",
				"--request", "suites/basic.yaml",
				"--output", "eval-results")
			command.Env = append(os.Environ(), "BENCH_CHILD_ARGS="+capture)
			if output, err := command.CombinedOutput(); err != nil {
				reportOnce.Do(func() {
					recordErr <- fmt.Errorf("run recorder: %w: %s", err, output)
				})
			}
		}()
	}
	writers.Wait()
	close(stopReader)
	<-readerDone
	select {
	case err := <-recordErr:
		t.Fatal(err)
	default:
	}
	requireBenchChildArgs(t, capture, child)
}

func TestBenchChildRecordReportsOmittedRequestWithIdentityAndArgv(t *testing.T) {
	runDir := t.TempDir()
	child := filepath.Join(runDir, "critic-child")
	capture := filepath.Join(runDir, "child-args.bin")
	writeBenchChildRecorder(t, child)
	command := exec.Command(child,
		"--profile", "agents/critic/profile.yaml",
		"--core-root", "/core",
		"--output", "eval-results")
	command.Env = append(os.Environ(), "BENCH_CHILD_ARGS="+capture)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run recorder: %v: %s", err, output)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	record, err := parseBenchChildRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	err = validateBenchChildRecord(record, child)
	if err == nil {
		t.Fatal("record without --request passed validation")
	}
	for _, want := range []string{
		"pid=", child, "--profile", "agents/critic/profile.yaml",
		"--core-root", "/core", "--output", "eval-results",
		`missing adjacent "--request" "suites/basic.yaml"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("diagnostic %q does not contain %q", err, want)
		}
	}
}

func requireBenchResponse(t *testing.T, url string, status int, contains string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", url, err)
	}
	if response.StatusCode != status || !strings.Contains(string(body), contains) {
		t.Fatalf("GET %s = %d %s, want %d containing %q", url, response.StatusCode, body, status, contains)
	}
}

func requireBenchTerminalState(t *testing.T, result RunResult, want string) {
	t.Helper()
	for _, span := range result.Spans {
		for _, event := range span.Events {
			if event.Name != TerminalEventName {
				continue
			}
			if state, _ := event.StringAttr("final_state"); state == want {
				return
			}
		}
	}
	t.Fatalf("no terminal event reached %q; spans: %v", want, result.Spans.Names())
}
