// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const imageLeaseVersion = 2

var imageLeaseSequence atomic.Uint64

// ImageLeaseClass is the ENG01 lifecycle class recorded on a lease.
type ImageLeaseClass string

const (
	RuntimeImageClass  ImageLeaseClass = "runtime"
	TestImageClass     ImageLeaseClass = "test"
	DerivedImageClass  ImageLeaseClass = "derived"
	CacheImageClass    ImageLeaseClass = "cache"
	UpstreamImageClass ImageLeaseClass = "upstream"
)

type imageLeaseOwner struct {
	PID      int       `json:"pid"`
	Acquired time.Time `json:"acquired"`
}

type imageLeaseState struct {
	Version     int                        `json:"version"`
	Class       ImageLeaseClass            `json:"class"`
	Reference   string                     `json:"reference"`
	ImageID     string                     `json:"image_id"`
	Revision    string                     `json:"revision"`
	Recipe      string                     `json:"recipe"`
	Platform    string                     `json:"platform"`
	PreExisting bool                       `json:"pre_existing"`
	Owners      map[string]imageLeaseOwner `json:"owners"`
	LastError   string                     `json:"last_error,omitempty"`
}

// AgentCoreImageLease holds one local owner of a canonical image tag. Release
// it only after the caller's owned cluster is deleted or every surviving
// cluster has loaded the image; the last owner removes only a tag this lease
// group created.
type AgentCoreImageLease struct {
	Result AgentCoreImageResult
	Owner  string

	manager *imageLeaseManager
}

type imageLeaseManager struct {
	root     string
	ensure   func(coreRoot, image, platform string) (AgentCoreImageResult, error)
	inspect  func(image string) (dockerImageMetadata, bool)
	run      func(args ...string) ([]byte, error)
	clusters func() ([]string, error)
	now      func() time.Time
	pid      func() int
	sleep    func(time.Duration)
}

func defaultImageLeaseManager() *imageLeaseManager {
	cache, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cache) == "" {
		cache = os.TempDir()
	}
	builder := defaultImageBuilder()
	return &imageLeaseManager{
		root: filepath.Join(cache, "declarative-agents", "image-leases"),
		ensure: func(coreRoot, image, platform string) (AgentCoreImageResult, error) {
			return builder.ensure(coreRoot, image, platform)
		},
		inspect: builder.inspectDockerImage,
		run: func(args ...string) ([]byte, error) {
			return exec.Command("docker", args...).CombinedOutput()
		},
		clusters: listKindClusters,
		now:      time.Now, pid: os.Getpid, sleep: time.Sleep,
	}
}

func listKindClusters() ([]string, error) {
	output, err := exec.Command("kind", "get", "clusters").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kind get clusters: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// ClassifyLeaseImage returns the lifecycle class and canonical name for a
// lease reference. Untyped, kindrig, and mutable names are rejected.
func ClassifyLeaseImage(ref string) (ImageLeaseClass, string, error) {
	ref = strings.TrimSpace(ref)
	if local, err := ParseLocal(ref); err == nil {
		return ImageLeaseClass(local.Role), local.String(), nil
	}
	if upstream, err := ParseUpstream(ref); err == nil {
		return UpstreamImageClass, upstream.String(), nil
	}
	return "", "", fmt.Errorf("image lease %q is not a typed local or fully qualified upstream reference", ref)
}

// ConfiguredUpstreamPins are the digest-pinned sources the rig currently
// installs. clean:images retains them; retired aliases are diagnosed instead.
func ConfiguredUpstreamPins() []string {
	raw := []string{CLIDonorImage}
	for _, arch := range []string{"amd64", "arm64"} {
		if image, err := traefikImage(arch); err == nil {
			raw = append(raw, image)
		}
		if image, err := metricsServerImage(arch); err == nil {
			raw = append(raw, image)
		}
		if image, err := fakeGCSImage(arch); err == nil {
			raw = append(raw, image)
		}
	}
	var pins []string
	seen := map[string]bool{}
	for _, ref := range raw {
		normalized, err := NormalizeUpstream(ref)
		if err != nil || seen[normalized] {
			continue
		}
		seen[normalized] = true
		pins = append(pins, normalized)
	}
	return pins
}

// IsConfiguredUpstreamPin reports whether ref is a current digest-pinned
// source (tag or digest match). Retired aliases are not pins.
func IsConfiguredUpstreamPin(ref string) bool {
	parsed, err := ParseUpstream(ref)
	if err != nil {
		return false
	}
	for _, pin := range ConfiguredUpstreamPins() {
		candidate, err := ParseUpstream(pin)
		if err != nil {
			continue
		}
		if candidate.Registry != parsed.Registry || candidate.Path != parsed.Path {
			continue
		}
		if parsed.Tag != "" && candidate.Tag == parsed.Tag {
			return true
		}
		if parsed.Digest != "" && candidate.Digest == parsed.Digest {
			return true
		}
	}
	return false
}

// AcquireAgentCoreImageLease ensures the exact canonical image and records an
// active process owner under a cross-process lock. The returned result carries
// the immutable image ID used to guard last-owner deletion.
func AcquireAgentCoreImageLease(
	coreRoot, image, ownerPrefix string,
) (*AgentCoreImageLease, error) {
	return defaultImageLeaseManager().acquire(coreRoot, image, HostPlatform(), ownerPrefix)
}

// AcquireLocalImageLease records an owner for a derived or cache image that
// is already present on the host. Release it after the owning cluster is gone.
func AcquireLocalImageLease(image, ownerPrefix string) (*AgentCoreImageLease, error) {
	return defaultImageLeaseManager().acquireLocal(image, ownerPrefix)
}

func (m *imageLeaseManager) acquireLocal(
	image, ownerPrefix string,
) (*AgentCoreImageLease, error) {
	class, image, err := ClassifyLeaseImage(image)
	if err != nil {
		return nil, err
	}
	if class != DerivedImageClass && class != CacheImageClass {
		return nil, fmt.Errorf(
			"local image lease %q is %s, not derived or cache", image, class)
	}
	ownerPrefix = strings.TrimSpace(ownerPrefix)
	if ownerPrefix == "" {
		return nil, errors.New("image lease owner prefix is required")
	}
	unlock, err := m.lock(image)
	if err != nil {
		return nil, err
	}
	defer unlock()

	state, found, err := m.read(image)
	if err != nil {
		return nil, err
	}
	result, tagPreExisted, err := m.resolveLocal(image, class)
	if err != nil {
		return nil, err
	}
	return m.recordAcquire(image, ownerPrefix, state, found, result, tagPreExisted)
}

func (m *imageLeaseManager) acquire(
	coreRoot, image, platform, ownerPrefix string,
) (*AgentCoreImageLease, error) {
	class, image, err := ClassifyLeaseImage(image)
	if err != nil {
		return nil, err
	}
	ownerPrefix = strings.TrimSpace(ownerPrefix)
	if ownerPrefix == "" {
		return nil, errors.New("image lease owner prefix is required")
	}
	unlock, err := m.lock(image)
	if err != nil {
		return nil, err
	}
	defer unlock()

	state, found, err := m.read(image)
	if err != nil {
		return nil, err
	}
	result, tagPreExisted, err := m.resolve(coreRoot, image, platform, class)
	if err != nil {
		return nil, err
	}
	return m.recordAcquire(image, ownerPrefix, state, found, result, tagPreExisted)
}

func (m *imageLeaseManager) recordAcquire(
	image, ownerPrefix string,
	state imageLeaseState, found bool,
	result AgentCoreImageResult, tagPreExisted bool,
) (*AgentCoreImageLease, error) {
	class, _, err := ClassifyLeaseImage(image)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.ImageID) == "" {
		return nil, fmt.Errorf("ensured image %s has no image ID", image)
	}
	if found && (state.Reference != result.Reference || state.ImageID != result.ImageID ||
		state.Revision != result.Revision || state.Recipe != result.Recipe ||
		state.Platform != result.Platform) {
		return nil, fmt.Errorf(
			"image lease %s records %s (%s) but Docker now resolves %s (%s); "+
				"active owners must release or recover the lease explicitly",
			m.path(image), state.Reference, state.ImageID, result.Reference, result.ImageID)
	}
	if !found {
		state = imageLeaseState{
			Version: imageLeaseVersion, Class: class, Reference: result.Reference,
			ImageID: result.ImageID, Revision: result.Revision,
			Recipe: result.Recipe, Platform: result.Platform,
			PreExisting: tagPreExisted, Owners: map[string]imageLeaseOwner{},
		}
	}
	if state.Class == "" {
		state.Class = class
	}
	if state.Owners == nil {
		state.Owners = map[string]imageLeaseOwner{}
	}
	owner := fmt.Sprintf("%s-p%d-%d-%d", ownerPrefix, m.pid(),
		m.now().UTC().UnixNano(), imageLeaseSequence.Add(1))
	state.Owners[owner] = imageLeaseOwner{PID: m.pid(), Acquired: m.now().UTC()}
	state.LastError = ""
	if err := m.write(image, state); err != nil {
		return nil, err
	}
	return &AgentCoreImageLease{Result: result, Owner: owner, manager: m}, nil
}

func (m *imageLeaseManager) resolveLocal(
	image string, class ImageLeaseClass,
) (AgentCoreImageResult, bool, error) {
	item, tagPreExisted := m.inspect(image)
	if !tagPreExisted {
		return AgentCoreImageResult{}, false, fmt.Errorf(
			"local image %s is not present on the host", image)
	}
	local, err := ParseLocal(image)
	if err != nil {
		return AgentCoreImageResult{}, tagPreExisted, err
	}
	platform := local.OS + "/" + local.Arch
	if item.OS+"/"+item.Architecture != platform {
		return AgentCoreImageResult{}, tagPreExisted, fmt.Errorf(
			"local image %s is %s/%s, want %s", image,
			item.OS, item.Architecture, platform)
	}
	result := AgentCoreImageResult{
		Reference: image,
		Revision:  local.Revision,
		Recipe:    local.Recipe,
		Platform:  platform,
		ImageID:   item.ID,
		Reused:    true,
	}
	if class == DerivedImageClass && local.Type != UpstreamIdentity {
		return AgentCoreImageResult{}, tagPreExisted, fmt.Errorf(
			"derived image %s does not carry upstream identity", image)
	}
	if class == CacheImageClass && local.Type != RecipeIdentity {
		return AgentCoreImageResult{}, tagPreExisted, fmt.Errorf(
			"cache image %s does not carry recipe identity", image)
	}
	return result, tagPreExisted, nil
}

func (m *imageLeaseManager) resolve(
	coreRoot, image, platform string, class ImageLeaseClass,
) (AgentCoreImageResult, bool, error) {
	item, tagPreExisted := m.inspect(image)
	if class == UpstreamImageClass {
		if !tagPreExisted {
			return AgentCoreImageResult{}, false, fmt.Errorf("upstream image %s is not present on the host", image)
		}
		return AgentCoreImageResult{
			Reference: image, ImageID: item.ID, Platform: platform, Reused: true,
		}, true, nil
	}
	result, err := m.ensure(coreRoot, image, platform)
	return result, tagPreExisted, err
}

// Release drops this owner. The last owner deletes the tag only when it did
// not pre-exist, still resolves to the leased image ID, and no container uses
// that ID. Failures retain diagnosable state for explicit recovery.
func (l *AgentCoreImageLease) Release() error {
	if l == nil || l.manager == nil || l.Owner == "" {
		return nil
	}
	err := l.manager.release(l.Result.Reference, l.Owner)
	if err == nil {
		l.Owner = ""
	}
	return err
}

func (m *imageLeaseManager) release(reference, owner string) error {
	unlock, err := m.lock(reference)
	if err != nil {
		return err
	}
	defer unlock()
	state, found, err := m.read(reference)
	if err != nil || !found {
		return err
	}
	if _, exists := state.Owners[owner]; !exists {
		return fmt.Errorf("image lease %s has no owner %q", m.path(reference), owner)
	}
	delete(state.Owners, owner)
	if len(state.Owners) > 0 {
		return m.write(reference, state)
	}
	return m.releaseWithoutOwner(reference, state)
}

// ImageLeaseStatus reports active owners with a bounded diagnostic. It never
// expires or deletes them based on age.
func ImageLeaseStatus(reference string) (bool, string) {
	_, reference, err := ClassifyLeaseImage(reference)
	if err != nil {
		return false, ""
	}
	return defaultImageLeaseManager().status(reference)
}

func (m *imageLeaseManager) status(reference string) (bool, string) {
	unlock, err := m.lock(reference)
	if err != nil {
		return true, err.Error()
	}
	defer unlock()
	state, found, err := m.read(reference)
	if err != nil {
		return true, err.Error()
	}
	if !found || len(state.Owners) == 0 {
		return false, ""
	}
	names := make([]string, 0, len(state.Owners))
	for name := range state.Owners {
		names = append(names, name)
	}
	sort.Strings(names)
	const limit = 5
	suffix := ""
	if len(names) > limit {
		suffix = fmt.Sprintf(" and %d more", len(names)-limit)
		names = names[:limit]
	}
	return true, fmt.Sprintf("%d active owner(s) in %s: %s%s; "+
		"do not remove automatically—use clean:imageLeaseRecover after confirming the owners are dead",
		len(state.Owners), m.path(reference), strings.Join(names, ", "), suffix)
}

// RecoverAgentCoreImageLease is explicit interruption recovery. It ignores
// recorded owners, but retains the same image-ID and container-use checks as a
// normal last release.
func RecoverAgentCoreImageLease(reference string) error {
	_, reference, err := ClassifyLeaseImage(reference)
	if err != nil {
		return err
	}
	return defaultImageLeaseManager().recover(reference)
}

func (m *imageLeaseManager) recover(reference string) error {
	unlock, err := m.lock(reference)
	if err != nil {
		return err
	}
	defer unlock()
	state, found, err := m.read(reference)
	if err != nil || !found {
		return err
	}
	fmt.Printf("recovering image lease %s with %d recorded owner(s)\n",
		m.path(reference), len(state.Owners))
	return m.releaseWithoutOwner(reference, state)
}

func managedLocalImageClass(class ImageLeaseClass) bool {
	switch class {
	case RuntimeImageClass, TestImageClass, DerivedImageClass, CacheImageClass:
		return true
	}
	return false
}

func ownerClusterHint(owner string) string {
	if i := strings.LastIndex(owner, "-p"); i > 0 {
		return owner[:i]
	}
	return owner
}

func ownerClusterIsLive(clusters []string, hint string) bool {
	if !strings.HasPrefix(hint, "da-") {
		return true
	}
	for _, name := range clusters {
		if name == hint {
			return true
		}
	}
	return false
}

// ReconcileImageLeases drops owners whose kind cluster is gone, then finishes
// last-owner deletion for managed local images. It never expires a lease by age.
func ReconcileImageLeases() error {
	return defaultImageLeaseManager().reconcile()
}

func (m *imageLeaseManager) reconcile() error {
	if m.clusters == nil {
		return nil
	}
	clusters, err := m.clusters()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("list image leases: %w", err)
	}
	var failures []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var preview imageLeaseState
		data, err := os.ReadFile(filepath.Join(m.root, entry.Name()))
		if err != nil || json.Unmarshal(data, &preview) != nil || preview.Reference == "" {
			continue
		}
		if err := m.reconcileReference(preview.Reference, clusters); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (m *imageLeaseManager) reconcileReference(reference string, clusters []string) error {
	unlock, err := m.lock(reference)
	if err != nil {
		return err
	}
	defer unlock()
	state, found, err := m.read(reference)
	if err != nil || !found {
		return err
	}
	changed := false
	for name := range state.Owners {
		hint := ownerClusterHint(name)
		if !ownerClusterIsLive(clusters, hint) {
			delete(state.Owners, name)
			changed = true
		}
	}
	if len(state.Owners) > 0 {
		if changed {
			return m.write(reference, state)
		}
		return nil
	}
	return m.releaseWithoutOwner(reference, state)
}

func (m *imageLeaseManager) releaseWithoutOwner(reference string, state imageLeaseState) error {
	if state.PreExisting || !managedLocalImageClass(state.Class) {
		return m.remove(reference)
	}
	item, exists := m.inspect(reference)
	if !exists {
		return m.remove(reference)
	}
	if item.ID != state.ImageID {
		state.LastError = fmt.Sprintf("reference now resolves to %s, leased %s", item.ID, state.ImageID)
		_ = m.write(reference, state)
		return fmt.Errorf("refusing to remove %s: %s", reference, state.LastError)
	}
	containers, runErr := m.run("ps", "-aq", "--filter", "ancestor="+state.ImageID)
	if runErr != nil {
		state.LastError = fmt.Sprintf("inspect image containers: %v: %s",
			runErr, strings.TrimSpace(string(containers)))
		_ = m.write(reference, state)
		return errors.New(state.LastError)
	}
	if used := strings.Fields(string(containers)); len(used) > 0 {
		state.LastError = fmt.Sprintf("image %s is used by %d container(s)", state.ImageID, len(used))
		_ = m.write(reference, state)
		return fmt.Errorf("refusing to remove %s: %s", reference, state.LastError)
	}
	if output, runErr := m.run("image", "rm", reference); runErr != nil {
		state.LastError = fmt.Sprintf("docker image rm: %v: %s", runErr, strings.TrimSpace(string(output)))
		_ = m.write(reference, state)
		return fmt.Errorf("remove leased image %s: %s", reference, state.LastError)
	}
	return m.remove(reference)
}

func (m *imageLeaseManager) lock(reference string) (func(), error) {
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return nil, fmt.Errorf("create image lease root: %w", err)
	}
	path := m.path(reference) + ".lock"
	deadline := m.now().Add(10 * time.Minute)
	for {
		if err := os.Mkdir(path, 0o700); err == nil {
			return func() { _ = os.Remove(path) }, nil
		} else if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire image lease lock: %w", err)
		}
		if m.now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for image lease lock %s", path)
		}
		m.sleep(100 * time.Millisecond)
	}
}

func (m *imageLeaseManager) path(reference string) string {
	sum := sha256.Sum256([]byte(reference))
	return filepath.Join(m.root, fmt.Sprintf("%x.json", sum[:16]))
}

func (m *imageLeaseManager) read(reference string) (imageLeaseState, bool, error) {
	var state imageLeaseState
	data, err := os.ReadFile(m.path(reference))
	if os.IsNotExist(err) {
		return state, false, nil
	}
	if err != nil {
		return state, false, fmt.Errorf("read image lease: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, false, fmt.Errorf("decode image lease %s: %w", m.path(reference), err)
	}
	if state.Reference != reference {
		return state, false, fmt.Errorf("image lease %s has version/reference %d/%q",
			m.path(reference), state.Version, state.Reference)
	}
	if state.Version != 1 && state.Version != imageLeaseVersion {
		return state, false, fmt.Errorf("image lease %s has version/reference %d/%q",
			m.path(reference), state.Version, state.Reference)
	}
	if state.Class == "" {
		if class, _, err := ClassifyLeaseImage(state.Reference); err == nil {
			state.Class = class
		}
	}
	state.Version = imageLeaseVersion
	return state, true, nil
}

func (m *imageLeaseManager) write(reference string, state imageLeaseState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp := m.path(reference) + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return fmt.Errorf("write image lease: %w", err)
	}
	if err := os.Rename(temp, m.path(reference)); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("publish image lease: %w", err)
	}
	return nil
}

func (m *imageLeaseManager) remove(reference string) error {
	if err := os.Remove(m.path(reference)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove image lease: %w", err)
	}
	return nil
}
