package logs

import (
	"deploy-service/internal/domain"
	"os"
	"strings"
	"testing"
)

func TestNewLokiReaderFromEnvironmentRequiresExplicitBaseURL(t *testing.T) {
	oldValue, hadOldValue := os.LookupEnv("LOKI_BASE_URL")
	defer func() {
		if hadOldValue {
			_ = os.Setenv("LOKI_BASE_URL", oldValue)
			return
		}
		_ = os.Unsetenv("LOKI_BASE_URL")
	}()

	_ = os.Unsetenv("LOKI_BASE_URL")
	if reader := NewLokiReaderFromEnvironment(); reader != nil {
		t.Fatal("expected nil reader when LOKI_BASE_URL is unset")
	}

	_ = os.Setenv("LOKI_BASE_URL", "http://loki.monitoring.svc.cluster.local:3100")
	reader := NewLokiReaderFromEnvironment()
	if reader == nil {
		t.Fatal("expected reader when LOKI_BASE_URL is set")
	}
	if reader.baseURL != "http://loki.monitoring.svc.cluster.local:3100" {
		t.Fatalf("unexpected base URL %q", reader.baseURL)
	}
}

func TestBuildQueryIncludesNamespaceAndOptionalFilters(t *testing.T) {
	query := buildQuery("project-prj-1", domain.ProjectLogsRequest{StageSlug: "staging", Search: "boom"})
	fragments := []string{
		`namespace="project-prj-1"`,
		`stage="staging"`,
		`|= "boom"`,
	}
	for _, fragment := range fragments {
		if !strings.Contains(query, fragment) {
			t.Fatalf("expected query to contain %q, got %s", fragment, query)
		}
	}
}

func TestDetectLevelBestEffort(t *testing.T) {
	if got := detectLevel("panic: broken"); got != "error" {
		t.Fatalf("expected error, got %q", got)
	}
	if got := detectLevel("WARN cache miss"); got != "warn" {
		t.Fatalf("expected warn, got %q", got)
	}
}
