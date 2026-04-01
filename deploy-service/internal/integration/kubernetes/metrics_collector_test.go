//go:build !integration

package kubernetes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseCPUQuantity(t *testing.T) {
	t.Parallel()

	cases := map[string]float64{
		"250m": 0.25,
		"1":    1,
		"500n": 0.0000005,
		"800u": 0.0008,
	}

	for input, want := range cases {
		got := parseCPUQuantity(input)
		if got != want {
			t.Fatalf("parseCPUQuantity(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseByteQuantityToGB(t *testing.T) {
	t.Parallel()

	got := parseByteQuantityToGB("1024Mi")
	if got < 0.99 || got > 1.01 {
		t.Fatalf("expected about 1GiB in GB, got %v", got)
	}

	got = parseByteQuantityToGB("1Gi")
	if got < 0.99 || got > 1.01 {
		t.Fatalf("expected about 1GiB in GB, got %v", got)
	}
}

func TestRequestedPodResourcesFallsBackToRequestsAndLimits(t *testing.T) {
	t.Parallel()

	items := []metricsPodItem{
		{
			Status: struct {
				Phase     string `json:"phase"`
				StartTime string `json:"startTime"`
			}{Phase: "Running"},
			Spec: struct {
				Containers []struct {
					Resources struct {
						Requests struct {
							CPU    string `json:"cpu"`
							Memory string `json:"memory"`
						} `json:"requests"`
						Limits struct {
							CPU    string `json:"cpu"`
							Memory string `json:"memory"`
						} `json:"limits"`
					} `json:"resources"`
				} `json:"containers"`
			}{
				Containers: []struct {
					Resources struct {
						Requests struct {
							CPU    string `json:"cpu"`
							Memory string `json:"memory"`
						} `json:"requests"`
						Limits struct {
							CPU    string `json:"cpu"`
							Memory string `json:"memory"`
						} `json:"limits"`
					} `json:"resources"`
				}{
					{
						Resources: struct {
							Requests struct {
								CPU    string `json:"cpu"`
								Memory string `json:"memory"`
							} `json:"requests"`
							Limits struct {
								CPU    string `json:"cpu"`
								Memory string `json:"memory"`
							} `json:"limits"`
						}{
							Requests: struct {
								CPU    string `json:"cpu"`
								Memory string `json:"memory"`
							}{
								CPU:    "500m",
								Memory: "512Mi",
							},
						},
					},
					{
						Resources: struct {
							Requests struct {
								CPU    string `json:"cpu"`
								Memory string `json:"memory"`
							} `json:"requests"`
							Limits struct {
								CPU    string `json:"cpu"`
								Memory string `json:"memory"`
							} `json:"limits"`
						}{
							Limits: struct {
								CPU    string `json:"cpu"`
								Memory string `json:"memory"`
							}{
								CPU:    "250m",
								Memory: "256Mi",
							},
						},
					},
				},
			},
		},
	}

	cpu, memory := requestedPodResources(items)
	if cpu != 0.75 {
		t.Fatalf("expected cpu 0.75, got %v", cpu)
	}
	if memory <= 0.70 || memory >= 0.76 {
		t.Fatalf("expected memory around 0.75 GiB in GB, got %v", memory)
	}
}

func TestPodUptimeHoursSumsActivePods(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 21, 1, 0, 0, 0, time.UTC)
	items := []metricsPodItem{
		{
			Status: struct {
				Phase     string `json:"phase"`
				StartTime string `json:"startTime"`
			}{
				Phase:     "Running",
				StartTime: now.Add(-2 * time.Hour).Format(time.RFC3339),
			},
		},
		{
			Status: struct {
				Phase     string `json:"phase"`
				StartTime string `json:"startTime"`
			}{
				Phase:     "Pending",
				StartTime: now.Add(-30 * time.Minute).Format(time.RFC3339),
			},
		},
		{
			Status: struct {
				Phase     string `json:"phase"`
				StartTime string `json:"startTime"`
			}{
				Phase:     "Succeeded",
				StartTime: now.Add(-6 * time.Hour).Format(time.RFC3339),
			},
		},
	}

	got := podUptimeHours(items, now)
	if got != 2.5 {
		t.Fatalf("expected pod uptime 2.5h, got %v", got)
	}
}

func TestCurrentEgressDeltaFallsBackAcrossPrometheusQueries(t *testing.T) {
	t.Parallel()

	var seenQueries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		seenQueries = append(seenQueries, query)

		var payload any
		switch {
		case strings.Contains(query, "container_network_transmit_bytes_total"):
			payload = map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result":     []any{},
				},
			}
		case strings.Contains(query, "cilium_pod_egress_bytes_total"):
			payload = map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result": []map[string]any{
						{"value": []any{1, "1073741824"}},
					},
				},
			}
		default:
			payload = map[string]any{
				"status": "success",
				"data": map[string]any{
					"resultType": "vector",
					"result":     []any{},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	collector := &KubectlMetricsCollector{
		prometheusBaseURL: server.URL,
		prometheusQueries: defaultPrometheusEgressQueries(),
		window:            5 * time.Minute,
		client:            server.Client(),
	}

	got := collector.currentEgressDelta(context.Background(), "project-prj-1")
	if got != 1 {
		t.Fatalf("expected 1 GiB egress, got %v", got)
	}
	if len(seenQueries) < 4 {
		t.Fatalf("expected fallback queries to be attempted, got %#v", seenQueries)
	}
}

func TestCurrentEgressDeltaSendsAuthorizationHeaderToPrometheus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer prom-token" {
			t.Fatalf("expected Authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{"value": []any{1, "0"}},
				},
			},
		})
	}))
	defer server.Close()

	collector := &KubectlMetricsCollector{
		prometheusBaseURL:   server.URL,
		prometheusAuthToken: "prom-token",
		prometheusQueries: []string{
			`sum(increase(container_network_transmit_bytes_total{namespace="${namespace}",pod!=""}[${window}]))`,
		},
		window: 5 * time.Minute,
		client: server.Client(),
	}

	collector.currentEgressDelta(context.Background(), "project-prj-1")
}
