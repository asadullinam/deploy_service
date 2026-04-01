//go:build system

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestKubectlProvisionerProjectLifecycleAgainstK3s(t *testing.T) {
	if testing.Short() {
		t.Skip("skip system test in short mode")
	}
	if strings.TrimSpace(os.Getenv("KUBECONFIG")) == "" {
		t.Skip("set KUBECONFIG to run kubernetes system tests")
	}

	provisioner, ok := NewProvisionerFromEnvironment().(*KubectlProvisioner)
	if !ok {
		t.Fatal("expected kubectl provisioner from environment")
	}

	projectID := fmt.Sprintf("prj-system-%d", time.Now().UnixNano())
	hostNamespace := NamespaceForProject(projectID)
	stageSlug := "staging"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cleanupCancel()
		_ = provisioner.DeleteProjectEnvironment(cleanupCtx, projectID)
	}()

	appsBaseDomain, err := provisioner.CreateProjectEnvironment(ctx, projectID)
	if err != nil {
		t.Fatalf("CreateProjectEnvironment returned error: %v", err)
	}
	if appsBaseDomain != "" {
		t.Fatalf("expected empty apps base domain in system test, got %q", appsBaseDomain)
	}

	if out, err := provisioner.runKubectl(ctx, []string{"get", "namespace", hostNamespace, "-o", "name"}, nil); err != nil {
		t.Fatalf("expected host namespace to exist: %v", err)
	} else if strings.TrimSpace(out) != "namespace/"+hostNamespace {
		t.Fatalf("unexpected host namespace output: %q", out)
	}

	kubeconfig, err := provisioner.getProjectKubeconfigRaw(ctx, projectID)
	if err != nil {
		t.Fatalf("getProjectKubeconfigRaw returned error: %v", err)
	}
	if !strings.Contains(kubeconfig, "kind: Config") {
		t.Fatalf("expected kubeconfig payload, got %q", kubeconfig)
	}

	if err := provisioner.CreateStageEnvironment(ctx, projectID, stageSlug); err != nil {
		t.Fatalf("CreateStageEnvironment returned error: %v", err)
	}

	if out, err := provisioner.runKubectlInVCluster(ctx, projectID, []string{"get", "namespace", stageSlug, "-o", "name"}, nil); err != nil {
		t.Fatalf("expected stage namespace inside vcluster: %v", err)
	} else if strings.TrimSpace(out) != "namespace/"+stageSlug {
		t.Fatalf("unexpected stage namespace output: %q", out)
	}

	if err := provisioner.SuspendProjectEnvironment(ctx, projectID); err != nil {
		t.Fatalf("SuspendProjectEnvironment returned error: %v", err)
	}
	if err := provisioner.ResumeProjectEnvironment(ctx, projectID); err != nil {
		t.Fatalf("ResumeProjectEnvironment returned error: %v", err)
	}

	waitForVClusterNamespace(t, ctx, provisioner, projectID, "kube-system")

	if err := provisioner.DeleteStageEnvironment(ctx, projectID, stageSlug); err != nil {
		t.Fatalf("DeleteStageEnvironment returned error: %v", err)
	}
	waitForVClusterNamespaceDeletion(t, ctx, provisioner, projectID, stageSlug)

	if err := provisioner.DeleteProjectEnvironment(ctx, projectID); err != nil {
		t.Fatalf("DeleteProjectEnvironment returned error: %v", err)
	}
	waitForHostNamespaceDeletion(t, ctx, provisioner, hostNamespace)
}

func waitForVClusterNamespace(t *testing.T, ctx context.Context, provisioner *KubectlProvisioner, projectID, namespace string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		out, err := provisioner.runKubectlInVCluster(ctx, projectID, []string{"get", "namespace", namespace, "-o", "name"}, nil)
		if err == nil {
			if strings.TrimSpace(out) != "namespace/"+namespace {
				t.Fatalf("unexpected namespace output for %s: %q", namespace, out)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected vcluster namespace %s to become reachable: %v", namespace, err)
		}
		t.Logf("waiting for vcluster namespace %s to become reachable: %v", namespace, err)
		time.Sleep(2 * time.Second)
	}
}

func waitForVClusterNamespaceDeletion(t *testing.T, ctx context.Context, provisioner *KubectlProvisioner, projectID, namespace string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		_, err := provisioner.runKubectlInVCluster(ctx, projectID, []string{"get", "namespace", namespace, "-o", "name"}, nil)
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected vcluster namespace %s lookup to fail after deletion", namespace)
		}
		t.Logf("waiting for vcluster namespace %s to disappear after deletion", namespace)
		time.Sleep(2 * time.Second)
	}
}

func waitForHostNamespaceDeletion(t *testing.T, ctx context.Context, provisioner *KubectlProvisioner, hostNamespace string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		_, err := provisioner.runKubectl(ctx, []string{"get", "namespace", hostNamespace, "-o", "name"}, nil)
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("expected host namespace lookup to fail after deletion")
		}
		t.Logf("waiting for host namespace %s to disappear after deletion", hostNamespace)
		time.Sleep(2 * time.Second)
	}
}
