// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	kubernetesSecretLimit          = 1 << 20
	helmReleaseRequiredMargin      = 128 << 10
	helmReleaseBudget              = kubernetesSecretLimit - helmReleaseRequiredMargin
	helmReleaseProjectionAllowance = 64 << 10
	externalUIAssetConfigMapBudget = 900 << 10
)

type externalUIAssetFile struct {
	PackagePath string
	ArchivePath string
	MountedPath string
	Checksum    string
}

type externalUIAsset struct {
	Component     string
	Archive       string
	ConfigMapName string
	Checksum      string
	Files         []externalUIAssetFile
}

type helmReleaseMeasurements struct {
	ChartArchiveBytes    int
	ChartFilesBytes      int
	ChartEncodedBytes    int
	ManifestBytes        int
	ManifestEncodedBytes int
	ValuesBytes          int
	ValuesEncodedBytes   int
	ProjectedSecretBytes int
	BudgetBytes          int
	RequiredMarginBytes  int
}

func (m helmReleaseMeasurements) String() string {
	return fmt.Sprintf(
		"chart_archive(out-of-release)=%d chart_files(raw=%d encoded=%d) "+
			"manifest(raw=%d encoded=%d) values(raw=%d encoded=%d) "+
			"projected_secret=%d budget=%d required_margin=%d",
		m.ChartArchiveBytes, m.ChartFilesBytes, m.ChartEncodedBytes,
		m.ManifestBytes, m.ManifestEncodedBytes, m.ValuesBytes, m.ValuesEncodedBytes,
		m.ProjectedSecretBytes, m.BudgetBytes, m.RequiredMarginBytes,
	)
}

// externalizeUIAssets turns the two largest immutable duplicated release
// contributors into deterministic archives for releaseName. The source files
// are removed from the staged chart only after their package-inventory
// checksums have been verified and the archives have been written. Helm
// therefore stores neither the UI chart files nor rendered UI ConfigMaps, while
// the workloads mount the same inventory bytes through explicitly named
// out-of-release ConfigMaps.
func externalizeUIAssets(chart, releaseName string) ([]externalUIAsset, func(), error) {
	if strings.TrimSpace(releaseName) == "" {
		return nil, nil, fmt.Errorf("external UI asset release name is empty")
	}
	data, err := os.ReadFile(filepath.Join(chart, filepath.FromSlash(chatbotClosureProvenance)))
	if err != nil {
		return nil, nil, fmt.Errorf("read staged closure provenance: %w", err)
	}
	var provenance chatbotPackageProvenance
	if err := yaml.Unmarshal(data, &provenance); err != nil {
		return nil, nil, fmt.Errorf("decode staged closure provenance: %w", err)
	}
	dir, err := os.MkdirTemp("", "chatbot-mesh-external-assets-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	specs := []struct {
		component   string
		prefix      string
		mountedRoot string
	}{
		{component: "collector", prefix: "collector-ui/", mountedRoot: "/collector-ui/"},
		{
			component:   "observer",
			prefix:      "profiles/agents/observer/ui/dist/",
			mountedRoot: "/observer-ui/",
		},
	}
	assets := make([]externalUIAsset, 0, len(specs))
	for _, spec := range specs {
		asset, err := writeExternalUIAsset(
			chart, dir, provenance, releaseName, spec.component, spec.prefix, spec.mountedRoot)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		assets = append(assets, asset)
	}
	for _, relative := range []string{"collector-ui", "profiles/agents/observer/ui/dist"} {
		if err := os.RemoveAll(filepath.Join(chart, filepath.FromSlash(relative))); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("remove release-resident UI %s: %w", relative, err)
		}
	}
	return assets, cleanup, nil
}

func writeExternalUIAsset(
	chart, dir string,
	provenance chatbotPackageProvenance,
	releaseName, component, prefix, mountedRoot string,
) (externalUIAsset, error) {
	asset := externalUIAsset{Component: component}
	for _, file := range provenance.Files {
		if !strings.HasPrefix(file.PackagePath, prefix) {
			continue
		}
		relative := strings.TrimPrefix(file.PackagePath, prefix)
		asset.Files = append(asset.Files, externalUIAssetFile{
			PackagePath: file.PackagePath,
			ArchivePath: relative,
			MountedPath: mountedRoot + relative,
			Checksum:    file.Checksum,
		})
	}
	if len(asset.Files) == 0 {
		return asset, fmt.Errorf("staged provenance contains no %s UI files below %s", component, prefix)
	}
	sort.Slice(asset.Files, func(i, j int) bool {
		return asset.Files[i].ArchivePath < asset.Files[j].ArchivePath
	})
	asset.Archive = filepath.Join(dir, component+"-ui.tgz")
	if err := writeDeterministicAssetArchive(chart, asset); err != nil {
		return asset, err
	}
	archive, err := os.ReadFile(asset.Archive)
	if err != nil {
		return asset, err
	}
	sum := sha256.Sum256(archive)
	asset.Checksum = hex.EncodeToString(sum[:])
	if len(archive) > externalUIAssetConfigMapBudget {
		return asset, fmt.Errorf("%s UI archive is %d bytes, over ConfigMap budget %d",
			component, len(archive), externalUIAssetConfigMapBudget)
	}
	asset.ConfigMapName = fmt.Sprintf("%s-%s-ui-%s",
		releaseName, component, asset.Checksum[:12])
	return asset, nil
}

func writeDeterministicAssetArchive(chart string, asset externalUIAsset) (result error) {
	file, err := os.Create(asset.Archive)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); result == nil && closeErr != nil {
			result = closeErr
		}
	}()
	gz, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	gz.Header.ModTime = time.Unix(0, 0)
	gz.Header.OS = 255
	tw := tar.NewWriter(gz)
	var checksumManifest bytes.Buffer
	for _, inventoryFile := range asset.Files {
		data, err := os.ReadFile(filepath.Join(chart, filepath.FromSlash(inventoryFile.PackagePath)))
		if err != nil {
			return fmt.Errorf("read %s UI inventory file %s: %w",
				asset.Component, inventoryFile.PackagePath, err)
		}
		sum := sha256.Sum256(data)
		actual := "sha256:" + hex.EncodeToString(sum[:])
		if actual != inventoryFile.Checksum {
			return fmt.Errorf("%s UI inventory checksum mismatch for %s: %s != %s",
				asset.Component, inventoryFile.PackagePath, actual, inventoryFile.Checksum)
		}
		header := &tar.Header{
			Name: inventoryFile.ArchivePath, Mode: 0o444, Size: int64(len(data)),
			ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
			Uid: 0, Gid: 0,
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(&checksumManifest, "%s  %s\n",
			strings.TrimPrefix(inventoryFile.Checksum, "sha256:"),
			inventoryFile.ArchivePath); err != nil {
			return err
		}
	}
	inventoryData := checksumManifest.Bytes()
	if err := tw.WriteHeader(&tar.Header{
		Name: ".inventory.sha256", Mode: 0o444, Size: int64(len(inventoryData)),
		ModTime: time.Unix(0, 0), AccessTime: time.Unix(0, 0), ChangeTime: time.Unix(0, 0),
		Uid: 0, Gid: 0,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(inventoryData); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func externalUIAssetValueArgs(assets []externalUIAsset) []string {
	args := make([]string, 0, len(assets)*4)
	for _, asset := range assets {
		args = append(args,
			"--set", asset.Component+".uiArchiveConfigMap="+asset.ConfigMapName,
			"--set-string", asset.Component+".uiArchiveChecksum="+asset.Checksum,
		)
	}
	return args
}

type helmReleaseBudgetFile struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

func measureHelmReleaseBudget(
	releaseName, chartPath, chartArchive string,
	valueArgs []string,
) (helmReleaseMeasurements, error) {
	var measured helmReleaseMeasurements
	if strings.TrimSpace(releaseName) == "" {
		return measured, fmt.Errorf("Helm release budget name is empty")
	}
	archiveData, err := os.ReadFile(chartArchive)
	if err != nil {
		return measured, err
	}
	measured.ChartArchiveBytes = len(archiveData)
	renderArgs := append([]string{"template", releaseName, chartPath}, valueArgs...)
	manifest, err := exec.Command("helm", renderArgs...).CombinedOutput()
	if err != nil {
		return measured, fmt.Errorf("render Helm release budget for %s: %w: %s",
			releaseName, err, strings.TrimSpace(string(manifest)))
	}
	if bytes.Contains(manifest, []byte("chart.tgz:")) ||
		bytes.Contains(manifest, []byte(base64.StdEncoding.EncodeToString(archiveData))) {
		return measured, fmt.Errorf("rendered release contains the out-of-release chart archive")
	}
	chartFiles, err := releaseBudgetChartFiles(chartArchive)
	if err != nil {
		return measured, err
	}
	chartJSON, err := json.Marshal(chartFiles)
	if err != nil {
		return measured, err
	}
	valuesPayload, err := releaseBudgetValuesPayload(valueArgs)
	if err != nil {
		return measured, err
	}
	measured.ChartFilesBytes = 0
	for _, file := range chartFiles {
		measured.ChartFilesBytes += len(file.Data)
	}
	measured.ManifestBytes = len(manifest)
	measured.ValuesBytes = len(valuesPayload)
	measured.ChartEncodedBytes, err = helmEncodedSize(chartJSON)
	if err != nil {
		return measured, err
	}
	measured.ManifestEncodedBytes, err = helmEncodedSize(manifest)
	if err != nil {
		return measured, err
	}
	measured.ValuesEncodedBytes, err = helmEncodedSize(valuesPayload)
	if err != nil {
		return measured, err
	}
	projection, err := helmReleaseProjection(releaseName, chartFiles, valuesPayload, manifest)
	if err != nil {
		return measured, err
	}
	projected, err := helmEncodedSize(projection)
	if err != nil {
		return measured, err
	}
	measured.ProjectedSecretBytes = projected + helmReleaseProjectionAllowance
	measured.BudgetBytes = helmReleaseBudget
	measured.RequiredMarginBytes = helmReleaseRequiredMargin
	if measured.ProjectedSecretBytes > measured.BudgetBytes {
		return measured, fmt.Errorf(
			"projected Helm release Secret exceeds safe budget before install: %s",
			measured.String())
	}
	return measured, nil
}

func helmReleaseProjection(
	releaseName string,
	chartFiles []helmReleaseBudgetFile,
	valuesPayload, manifest []byte,
) ([]byte, error) {
	return json.Marshal(struct {
		Name      string                  `json:"name"`
		Namespace string                  `json:"namespace"`
		Version   int                     `json:"version"`
		Chart     []helmReleaseBudgetFile `json:"chart"`
		Config    string                  `json:"config"`
		Manifest  string                  `json:"manifest"`
	}{
		Name: releaseName, Namespace: "default", Version: 1,
		Chart: chartFiles, Config: string(valuesPayload), Manifest: string(manifest),
	})
}

func releaseBudgetChartFiles(archive string) ([]helmReleaseBudgetFile, error) {
	file, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	var files []helmReleaseBudgetFile
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		files = append(files, helmReleaseBudgetFile{Name: header.Name, Data: data})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func releaseBudgetValuesPayload(args []string) ([]byte, error) {
	var payload bytes.Buffer
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--values":
			index++
			data, err := os.ReadFile(args[index])
			if err != nil {
				return nil, err
			}
			payload.Write(data)
		case "--set", "--set-string":
			index++
			payload.WriteString(args[index])
			payload.WriteByte('\n')
		}
	}
	return payload.Bytes(), nil
}

// helmEncodedSize mirrors Helm's storage driver: release JSON is gzipped at
// best compression, then base64-encoded into Secret.Data["release"]. Kubernetes
// applies a second base64 layer on the wire, but validates the decoded Data
// value, so this inner encoded length is the 1 MiB quantity.
func helmEncodedSize(data []byte) (int, error) {
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	if _, err := writer.Write(data); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	return base64.StdEncoding.EncodedLen(compressed.Len()), nil
}

func provisionExternalUIAssetWithRunner(
	run helmLLMCommandRunner,
	asset externalUIAsset,
) error {
	if out, err := run(
		"kubectl", "delete", "configmap", asset.ConfigMapName, "--ignore-not-found"); err != nil {
		return fmt.Errorf("clear stale %s UI ConfigMap %s: %w: %s",
			asset.Component, asset.ConfigMapName, err, strings.TrimSpace(string(out)))
	}
	if out, err := run("kubectl", "create", "configmap", asset.ConfigMapName,
		"--from-file=assets.tgz="+asset.Archive); err != nil {
		return fmt.Errorf("create %s UI ConfigMap %s: %w: %s",
			asset.Component, asset.ConfigMapName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func provisionExternalUIAsset(asset externalUIAsset) error {
	return provisionExternalUIAssetWithRunner(runHelmSmokeCommand, asset)
}

func assertExternalUIAssetsMountedWithRunner(
	run helmLLMCommandRunner,
	releaseName string,
	assets []externalUIAsset,
	readyWait time.Duration,
) error {
	for _, asset := range assets {
		deployment := "deployment/" + releaseName + "-chatbot-mesh-" + asset.Component
		selector := "app.kubernetes.io/instance=" + releaseName +
			",app.kubernetes.io/component=" + asset.Component
		out, err := run("kubectl", "wait", "pod", "-l", selector,
			"--for=jsonpath={.status.phase}=Running",
			"--timeout", readyWait.String())
		if err != nil {
			return fmt.Errorf("%s did not start for mounted asset verification: %w: %s",
				deployment, err, strings.TrimSpace(string(out)))
		}
		jsonPath := fmt.Sprintf(
			`jsonpath={.items[0].status.initContainerStatuses[?(@.name=="stage-%s-ui")].state.terminated.exitCode}`,
			asset.Component)
		out, err = run("kubectl", "get", "pod", "-l", selector, "-o", jsonPath)
		if err != nil {
			return fmt.Errorf("read %s mounted asset verification: %w: %s",
				asset.Component, err, strings.TrimSpace(string(out)))
		}
		if strings.TrimSpace(string(out)) != "0" {
			return fmt.Errorf("%s mounted asset verifier exit = %q, want 0",
				asset.Component, strings.TrimSpace(string(out)))
		}
		if err := assertAssetContainerStable(run, selector, asset.Component); err != nil {
			return err
		}
		fmt.Printf("%s: mounted %s UI matches %d manifest-inventory files (%s)\n",
			releaseName, asset.Component, len(asset.Files), asset.Checksum)
	}
	return nil
}

func assertExternalUIAssetsMounted(
	releaseName string,
	assets []externalUIAsset,
	readyWait time.Duration,
) error {
	return assertExternalUIAssetsMountedWithRunner(
		runHelmSmokeCommand, releaseName, assets, readyWait)
}

func assertAssetContainerStable(
	run helmLLMCommandRunner,
	selector, component string,
) error {
	time.Sleep(3 * time.Second)
	out, err := run("kubectl", "get", "pod", "-l", selector, "-o", "json")
	if err != nil {
		return fmt.Errorf("inspect %s pod after asset staging: %w: %s",
			component, err, strings.TrimSpace(string(out)))
	}
	var list struct {
		Items []struct {
			Status struct {
				ContainerStatuses []struct {
					Name         string `json:"name"`
					RestartCount int    `json:"restartCount"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return err
	}
	if len(list.Items) == 0 {
		return fmt.Errorf("no %s pod found after asset staging", component)
	}
	for _, status := range list.Items[0].Status.ContainerStatuses {
		if status.Name != component || status.RestartCount == 0 {
			continue
		}
		logs, _ := run("kubectl", "logs", "-l", selector,
			"-c", component, "--previous", "--tail=100")
		return fmt.Errorf("%s restarted after asset staging (%d restarts): %s",
			component, status.RestartCount, strings.TrimSpace(string(logs)))
	}
	return nil
}

func assertHelmReleaseSecretsWithRunner(
	run helmLLMCommandRunner,
	releaseName, chartArchive string,
	assets []externalUIAsset,
) error {
	archive, err := os.ReadFile(chartArchive)
	if err != nil {
		return err
	}
	forbidden := [][]byte{[]byte(base64.StdEncoding.EncodeToString(archive))}
	for _, asset := range assets {
		data, err := os.ReadFile(asset.Archive)
		if err != nil {
			return err
		}
		forbidden = append(forbidden, []byte(base64.StdEncoding.EncodeToString(data)))
	}
	out, err := run("kubectl", "get", "secret",
		"-l", "owner=helm,name="+releaseName, "-o", "json")
	if err != nil {
		return fmt.Errorf("read Helm release Secrets: %w: %s", err, strings.TrimSpace(string(out)))
	}
	count, largest, err := inspectHelmReleaseSecrets(releaseName, out, forbidden)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d Helm release Secrets checked; largest data.release=%d bytes, margin=%d bytes\n",
		releaseName, count, largest, kubernetesSecretLimit-largest)
	return nil
}

func assertHelmReleaseSecrets(
	releaseName, chartArchive string,
	assets []externalUIAsset,
) error {
	return assertHelmReleaseSecretsWithRunner(
		runHelmSmokeCommand, releaseName, chartArchive, assets)
}

func inspectHelmReleaseSecrets(
	releaseName string,
	secretList []byte,
	forbidden [][]byte,
) (int, int, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Data map[string]string `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(secretList, &list); err != nil {
		return 0, 0, fmt.Errorf("decode Helm release Secret list: %w", err)
	}
	if len(list.Items) == 0 {
		return 0, 0, fmt.Errorf("no Helm release Secrets found for %s", releaseName)
	}
	largest := 0
	for _, item := range list.Items {
		stored, err := base64.StdEncoding.DecodeString(item.Data["release"])
		if err != nil {
			return 0, 0, fmt.Errorf("decode Kubernetes Secret %s: %w", item.Metadata.Name, err)
		}
		if len(stored) > largest {
			largest = len(stored)
		}
		if len(stored) > helmReleaseBudget {
			return 0, 0, fmt.Errorf("Helm release Secret %s data.release is %d bytes, over safe budget %d",
				item.Metadata.Name, len(stored), helmReleaseBudget)
		}
		releaseJSON, err := decodeHelmRelease(stored)
		if err != nil {
			return 0, 0, fmt.Errorf("decode Helm release %s: %w", item.Metadata.Name, err)
		}
		for _, encoded := range forbidden {
			if bytes.Contains(releaseJSON, encoded) {
				return 0, 0, fmt.Errorf(
					"Helm release %s stores an out-of-release archive", item.Metadata.Name)
			}
		}
		if bytes.Contains(releaseJSON, []byte("chart.tgz:")) {
			return 0, 0, fmt.Errorf("Helm release %s stores chart archive bytes", item.Metadata.Name)
		}
	}
	return len(list.Items), largest, nil
}

func decodeHelmRelease(stored []byte) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(string(stored))
	if err != nil {
		return nil, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}
