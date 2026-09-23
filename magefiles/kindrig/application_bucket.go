// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ApplicationBucketRequest identifies one local fake-GCS bucket through the
// same explicit cluster credential used by application namespace mechanics.
// Cloud provisioning uses provider IAM and is intentionally a separate
// implementation of the same resolved bucket identity.
type ApplicationBucketRequest struct {
	Cluster        string
	KubeconfigPath string
	BucketURL      string
}

// EnsureFakeGCSApplicationBucket creates one manifest-resolved application
// bucket when absent and never empties or replaces an existing bucket. The bool
// reports whether this invocation created it.
func EnsureFakeGCSApplicationBucket(request ApplicationBucketRequest) (bool, error) {
	bucket, err := applicationBucketName(request.BucketURL)
	if err != nil {
		return false, err
	}
	run, cleanup, err := applicationNamespaceRunner(ApplicationNamespaceRequest{
		Cluster: request.Cluster, KubeconfigPath: request.KubeconfigPath, Namespace: fakeGCSNamespace,
	})
	if err != nil {
		return false, err
	}
	defer cleanup()
	return ensureFakeGCSBucketWithRunner(run, bucket)
}

func ensureFakeGCSBucketWithRunner(run CommandRunner, bucket string) (bool, error) {
	if exists, statusErr := fakeGCSBucketExists(run, bucket); statusErr != nil {
		return false, statusErr
	} else if exists {
		return false, nil
	}
	body := fmt.Sprintf(`{"name":%q}`, bucket)
	output, err := fakeGCSWget(run,
		"--header=Content-Type: application/json", "--post-data="+body,
		"http://127.0.0.1:4443/storage/v1/b?project=apprig")
	if err != nil {
		// A concurrent ensure may have won after our absent read.
		if exists, retryErr := fakeGCSBucketExists(run, bucket); retryErr == nil && exists {
			return false, nil
		}
		return false, fmt.Errorf("create application bucket %s: %w: %s",
			bucket, err, strings.TrimSpace(string(output)))
	}
	return true, nil
}

// FakeGCSApplicationBucketStatus reports whether the resolved local bucket
// exists. It performs no mutation.
func FakeGCSApplicationBucketStatus(request ApplicationBucketRequest) (bool, error) {
	bucket, err := applicationBucketName(request.BucketURL)
	if err != nil {
		return false, err
	}
	run, cleanup, err := applicationNamespaceRunner(ApplicationNamespaceRequest{
		Cluster: request.Cluster, KubeconfigPath: request.KubeconfigPath, Namespace: fakeGCSNamespace,
	})
	if err != nil {
		return false, err
	}
	defer cleanup()
	return fakeGCSBucketExists(run, bucket)
}

// FakeGCSApplicationBucketObjects lists retained object keys without changing
// them, so app:down and redeploy proofs can compare durable identity.
func FakeGCSApplicationBucketObjects(request ApplicationBucketRequest) ([]string, error) {
	bucket, err := applicationBucketName(request.BucketURL)
	if err != nil {
		return nil, err
	}
	run, cleanup, err := applicationNamespaceRunner(ApplicationNamespaceRequest{
		Cluster: request.Cluster, KubeconfigPath: request.KubeconfigPath, Namespace: fakeGCSNamespace,
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return fakeGCSBucketObjectsWithRunner(run, bucket)
}

func fakeGCSBucketObjectsWithRunner(run CommandRunner, bucket string) ([]string, error) {
	output, err := fakeGCSWget(run, "http://127.0.0.1:4443/storage/v1/b/"+url.PathEscape(bucket)+"/o")
	if err != nil {
		return nil, fmt.Errorf("list application bucket %s: %w: %s",
			bucket, err, strings.TrimSpace(string(output)))
	}
	var response struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("decode application bucket %s listing: %w", bucket, err)
	}
	keys := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		keys = append(keys, item.Name)
	}
	return keys, nil
}

func applicationBucketName(bucketURL string) (string, error) {
	parsed, err := url.Parse(bucketURL)
	if err != nil || parsed.Scheme != "gs" || parsed.Host == "" ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("local application bucket URL %q must be gs://<bucket> with no path or query", bucketURL)
	}
	return parsed.Host, nil
}

func fakeGCSBucketExists(run CommandRunner, bucket string) (bool, error) {
	output, err := fakeGCSWget(run, "http://127.0.0.1:4443/storage/v1/b/"+url.PathEscape(bucket))
	if err == nil {
		return true, nil
	}
	if strings.Contains(string(output), "404 Not Found") {
		return false, nil
	}
	return false, fmt.Errorf("read application bucket %s: %w: %s",
		bucket, err, strings.TrimSpace(string(output)))
}

func fakeGCSWget(run CommandRunner, args ...string) ([]byte, error) {
	command := []string{"exec", "--namespace", fakeGCSNamespace, "deployment/" + fakeGCSDeployment,
		"--", "wget", "-q", "-S", "-O", "-"}
	command = append(command, args...)
	return run("kubectl", command...)
}
