package kubernetes

import (
	"bytes"
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var _ service.MetricsCollector = (*KubectlMetricsCollector)(nil)

func NewMetricsCollectorFromEnvironment(interval time.Duration) service.MetricsCollector {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("METRICS_COLLECTOR_MODE")))
	provisionerMode := strings.ToLower(strings.TrimSpace(os.Getenv("KUBERNETES_PROVISIONER")))
	if mode == "mock" || (provisionerMode != "" && provisionerMode != "kubectl") {
		return &MetricsCollectorMock{}
	}

	kubectlPath := strings.TrimSpace(os.Getenv("KUBECTL_PATH"))
	if kubectlPath == "" {
		kubectlPath = "kubectl"
	}

	return &KubectlMetricsCollector{
		kubectlPath:         kubectlPath,
		contextName:         strings.TrimSpace(os.Getenv("KUBECTL_CONTEXT")),
		prometheusBaseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("PROMETHEUS_BASE_URL")), "/"),
		prometheusAuthToken: strings.TrimSpace(os.Getenv("PROMETHEUS_AUTH_TOKEN")),
		prometheusQueries:   prometheusEgressQueriesFromEnvironment(),
		window:              interval,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *KubectlMetricsCollector) CollectProjectUsage(ctx context.Context, projectID string) (domain.ResourceSnapshot, error) {
	namespace := NamespaceForProject(projectID)

	pods, err := c.listPods(ctx, namespace)
	if err != nil {
		return domain.ResourceSnapshot{}, err
	}

	replicas, err := c.currentReplicaCount(ctx, namespace)
	if err != nil {
		return domain.ResourceSnapshot{}, err
	}
	if replicas == 0 {
		replicas = countActivePods(pods.Items)
	}

	cpuCores, memoryGB := c.currentPodUsageFromMetricsAPI(ctx, namespace)
	if cpuCores == 0 && memoryGB == 0 {
		cpuCores, memoryGB = requestedPodResources(pods.Items)
	}

	storageGB, err := c.currentPVCStorage(ctx, namespace)
	if err != nil {
		return domain.ResourceSnapshot{}, err
	}

	return domain.ResourceSnapshot{
		CPUCores:       cpuCores,
		MemoryGB:       memoryGB,
		StorageGB:      storageGB,
		EgressGBDelta:  c.currentEgressDelta(ctx, namespace),
		ReplicaCount:   replicas,
		PodUptimeHours: podUptimeHours(pods.Items, time.Now().UTC()),
	}, nil
}

func (c *KubectlMetricsCollector) currentReplicaCount(ctx context.Context, namespace string) (int32, error) {
	out, err := c.runKubectl(ctx, []string{"get", "deployment", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Items []struct {
			Status struct {
				Replicas int32 `json:"replicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return 0, fmt.Errorf("decode deployments for metrics: %w", err)
	}
	var replicas int32
	for _, item := range payload.Items {
		replicas += item.Status.Replicas
	}
	return replicas, nil
}

func (c *KubectlMetricsCollector) listPods(ctx context.Context, namespace string) (metricsPodList, error) {
	out, err := c.runKubectl(ctx, []string{"get", "pods", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return metricsPodList{}, err
	}
	var payload metricsPodList
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return metricsPodList{}, fmt.Errorf("decode pods for metrics: %w", err)
	}
	return payload, nil
}

func (c *KubectlMetricsCollector) currentPVCStorage(ctx context.Context, namespace string) (float64, error) {
	out, err := c.runKubectl(ctx, []string{"get", "pvc", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return 0, err
	}
	var payload metricsPVCList
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return 0, fmt.Errorf("decode pvcs for metrics: %w", err)
	}
	total := 0.0
	for _, item := range payload.Items {
		quantity := item.Status.Capacity.Storage
		if quantity == "" {
			quantity = item.Spec.Resources.Requests.Storage
		}
		total += parseByteQuantityToGB(quantity)
	}
	return total, nil
}

func (c *KubectlMetricsCollector) currentPodUsageFromMetricsAPI(ctx context.Context, namespace string) (float64, float64) {
	out, err := c.runKubectl(ctx, []string{"get", "--raw", fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods", namespace)}, nil)
	if err != nil {
		return 0, 0
	}

	var payload metricsPodUsageList
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return 0, 0
	}

	totalCPU := 0.0
	totalMemoryGB := 0.0
	for _, item := range payload.Items {
		for _, container := range item.Containers {
			totalCPU += parseCPUQuantity(container.Usage.CPU)
			totalMemoryGB += parseByteQuantityToGB(container.Usage.Memory)
		}
	}
	return totalCPU, totalMemoryGB
}

func (c *KubectlMetricsCollector) currentEgressDelta(ctx context.Context, namespace string) float64 {
	if c.prometheusBaseURL == "" {
		return 0
	}

	for _, queryTemplate := range c.prometheusQueries {
		query := strings.ReplaceAll(queryTemplate, "${namespace}", namespace)
		query = strings.ReplaceAll(query, "${window}", prometheusRangeWindow(c.window))
		bytesValue, ok := c.queryPrometheusVectorValue(ctx, query)
		if !ok {
			continue
		}
		return bytesValue / (1024 * 1024 * 1024)
	}

	return 0
}

func (c *KubectlMetricsCollector) queryPrometheusVectorValue(ctx context.Context, query string) (float64, bool) {
	u, err := url.Parse(c.prometheusBaseURL + "/api/v1/query")
	if err != nil {
		return 0, false
	}
	values := u.Query()
	values.Set("query", query)
	u.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, false
	}
	if c.prometheusAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.prometheusAuthToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, false
	}

	var payload struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, false
	}
	if payload.Status != "success" || payload.Data.ResultType != "vector" || len(payload.Data.Result) == 0 || len(payload.Data.Result[0].Value) < 2 {
		return 0, false
	}

	value, _ := payload.Data.Result[0].Value[1].(string)
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func (c *KubectlMetricsCollector) runKubectl(ctx context.Context, args []string, stdin []byte) (string, error) {
	fullArgs := make([]string, 0, len(args)+2)
	if c.contextName != "" {
		fullArgs = append(fullArgs, "--context", c.contextName)
	}
	fullArgs = append(fullArgs, args...)

	command := exec.CommandContext(ctx, c.kubectlPath, fullArgs...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return "", fmt.Errorf("kubectl %s failed: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func requestedPodResources(items []metricsPodItem) (float64, float64) {
	totalCPU := 0.0
	totalMemoryGB := 0.0
	for _, pod := range items {
		if !isBillablePodPhase(pod.Status.Phase) {
			continue
		}
		for _, container := range pod.Spec.Containers {
			cpu := strings.TrimSpace(container.Resources.Requests.CPU)
			if cpu == "" {
				cpu = strings.TrimSpace(container.Resources.Limits.CPU)
			}
			memory := strings.TrimSpace(container.Resources.Requests.Memory)
			if memory == "" {
				memory = strings.TrimSpace(container.Resources.Limits.Memory)
			}
			totalCPU += parseCPUQuantity(cpu)
			totalMemoryGB += parseByteQuantityToGB(memory)
		}
	}
	return totalCPU, totalMemoryGB
}

func podUptimeHours(items []metricsPodItem, now time.Time) float64 {
	total := 0.0
	for _, pod := range items {
		if !isBillablePodPhase(pod.Status.Phase) || pod.Status.StartTime == "" {
			continue
		}
		startTime, err := time.Parse(time.RFC3339, pod.Status.StartTime)
		if err != nil || startTime.After(now) {
			continue
		}
		total += now.Sub(startTime).Hours()
	}
	return total
}

func countActivePods(items []metricsPodItem) int32 {
	var total int32
	for _, pod := range items {
		if isBillablePodPhase(pod.Status.Phase) {
			total++
		}
	}
	return total
}

func isBillablePodPhase(phase string) bool {
	switch strings.TrimSpace(phase) {
	case "Pending", "Running":
		return true
	default:
		return false
	}
}

func parseCPUQuantity(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	switch {
	case strings.HasSuffix(value, "n"):
		return parseFloatWithSuffix(value, "n") / 1_000_000_000
	case strings.HasSuffix(value, "u"):
		return parseFloatWithSuffix(value, "u") / 1_000_000
	case strings.HasSuffix(value, "m"):
		return parseFloatWithSuffix(value, "m") / 1000
	default:
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0
		}
		return number
	}
}

func parseByteQuantityToGB(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	binaryUnits := map[string]float64{
		"Ki": math.Pow(1024, 1),
		"Mi": math.Pow(1024, 2),
		"Gi": math.Pow(1024, 3),
		"Ti": math.Pow(1024, 4),
		"Pi": math.Pow(1024, 5),
		"Ei": math.Pow(1024, 6),
	}
	for suffix, multiplier := range binaryUnits {
		if strings.HasSuffix(value, suffix) {
			return parseFloatWithSuffix(value, suffix) * multiplier / math.Pow(1024, 3)
		}
	}

	decimalUnits := map[string]float64{
		"K": 1_000,
		"M": 1_000_000,
		"G": 1_000_000_000,
		"T": 1_000_000_000_000,
		"P": 1_000_000_000_000_000,
		"E": 1_000_000_000_000_000_000,
	}
	for suffix, multiplier := range decimalUnits {
		if strings.HasSuffix(value, suffix) {
			return parseFloatWithSuffix(value, suffix) * multiplier / math.Pow(1024, 3)
		}
	}

	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return number / math.Pow(1024, 3)
}

func parseFloatWithSuffix(value string, suffix string) float64 {
	number, err := strconv.ParseFloat(strings.TrimSuffix(value, suffix), 64)
	if err != nil {
		return 0
	}
	return number
}

func prometheusRangeWindow(window time.Duration) string {
	if window < time.Minute {
		return fmt.Sprintf("%ds", int(window.Seconds()))
	}
	if window%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(window.Minutes()))
	}
	return fmt.Sprintf("%ds", int(window.Seconds()))
}

func prometheusEgressQueriesFromEnvironment() []string {
	raw := strings.TrimSpace(os.Getenv("PROMETHEUS_EGRESS_QUERIES"))
	if raw == "" {
		return defaultPrometheusEgressQueries()
	}

	parts := strings.Split(raw, "\n")
	queries := make([]string, 0, len(parts))
	for _, part := range parts {
		query := strings.TrimSpace(part)
		if query != "" {
			queries = append(queries, query)
		}
	}
	if len(queries) == 0 {
		return defaultPrometheusEgressQueries()
	}
	return queries
}

func defaultPrometheusEgressQueries() []string {
	return []string{
		`sum(increase(container_network_transmit_bytes_total{namespace="${namespace}",pod!=""}[${window}]))`,
		`sum(increase(container_network_egress_bytes_total{namespace="${namespace}",pod!=""}[${window}]))`,
		`sum(increase(pod_network_egress_bytes_total{namespace="${namespace}",pod!=""}[${window}]))`,
		`sum(increase(cilium_pod_egress_bytes_total{namespace="${namespace}"}[${window}]))`,
		`sum(increase(calico_pod_egress_bytes_total{namespace="${namespace}"}[${window}]))`,
	}
}
