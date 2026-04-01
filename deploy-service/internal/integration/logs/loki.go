package logs

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/integration/kubernetes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func NewLokiReaderFromEnvironment() *LokiReader {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("LOKI_BASE_URL")), "/")
	if baseURL == "" {
		return nil
	}
	return &LokiReader{
		baseURL: baseURL,
		client:  &http.Client{Timeout: defaultTimeout},
	}
}

func (r *LokiReader) ListProjectLogs(ctx context.Context, projectID string, request domain.ProjectLogsRequest) (domain.ProjectLogsResponse, error) {
	if strings.TrimSpace(projectID) == "" {
		return domain.ProjectLogsResponse{}, errors.New("project id is required")
	}
	if strings.TrimSpace(r.baseURL) == "" {
		return domain.ProjectLogsResponse{}, errors.New("loki is not configured")
	}

	namespace := kubernetes.NamespaceForProject(projectID)
	query := buildQuery(namespace, request)
	limit := request.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	since := 15 * time.Minute
	if request.Since != "" {
		if parsed, err := time.ParseDuration(request.Since); err == nil && parsed > 0 {
			since = parsed
		}
	}

	values := url.Values{}
	values.Set("query", query)
	values.Set("limit", strconv.Itoa(limit))
	values.Set("direction", "backward")
	values.Set("start", strconv.FormatInt(time.Now().Add(-since).UnixNano(), 10))
	values.Set("end", strconv.FormatInt(time.Now().UnixNano(), 10))

	u, err := url.Parse(r.baseURL + "/loki/api/v1/query_range")
	if err != nil {
		return domain.ProjectLogsResponse{}, fmt.Errorf("parse loki url: %w", err)
	}
	u.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return domain.ProjectLogsResponse{}, fmt.Errorf("build loki request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return domain.ProjectLogsResponse{}, fmt.Errorf("query loki: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.ProjectLogsResponse{}, fmt.Errorf("read loki response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return domain.ProjectLogsResponse{}, fmt.Errorf("loki api: %s", message)
	}

	var payload lokiQueryResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return domain.ProjectLogsResponse{}, fmt.Errorf("decode loki response: %w", err)
	}
	if payload.Status != "success" {
		if payload.Error == "" {
			payload.Error = "unexpected loki status"
		}
		return domain.ProjectLogsResponse{}, errors.New(payload.Error)
	}

	entries := flattenEntries(payload.Data.Result)
	if len(entries) > limit {
		entries = entries[:limit]
	}

	return domain.ProjectLogsResponse{
		ProjectID: projectID,
		Namespace: namespace,
		StageID:   request.StageID,
		StageSlug: request.StageSlug,
		Query:     query,
		Entries:   entries,
	}, nil
}

func buildQuery(namespace string, request domain.ProjectLogsRequest) string {
	selector := []string{fmt.Sprintf(`namespace=%q`, namespace)}
	pod := strings.TrimSpace(request.Pod)
	container := strings.TrimSpace(request.Container)
	level := strings.ToLower(strings.TrimSpace(request.Level))
	if stage := strings.TrimSpace(request.StageSlug); stage != "" {
		selector = append(selector, fmt.Sprintf(`stage=%q`, stage))
	}
	if pod != "" {
		selector = append(selector, fmt.Sprintf(`pod=%q`, pod))
	}
	if container != "" {
		selector = append(selector, fmt.Sprintf(`container=%q`, container))
	}
	query := "{" + strings.Join(selector, ",") + "}"
	if filter := strings.TrimSpace(request.Search); filter != "" {
		query += fmt.Sprintf(` |= %q`, filter)
	}
	switch level {
	case "error":
		query += ` |~ "(?i)(error|fatal|panic)"`
	case "warn", "warning":
		query += ` |~ "(?i)(warn|warning)"`
	}
	return query
}

func flattenEntries(results []lokiStreamResult) []domain.ProjectLogEntry {
	entries := make([]domain.ProjectLogEntry, 0)
	for _, stream := range results {
		pod := strings.TrimSpace(stream.Stream["pod"])
		container := strings.TrimSpace(stream.Stream["container"])
		stage := strings.TrimSpace(stream.Stream["stage"])
		for _, item := range stream.Values {
			if len(item) < 2 {
				continue
			}
			ts, err := strconv.ParseInt(item[0], 10, 64)
			if err != nil {
				continue
			}
			message := item[1]
			entries = append(entries, domain.ProjectLogEntry{
				Timestamp: time.Unix(0, ts).UTC(),
				Pod:       pod,
				Container: container,
				Stage:     stage,
				Level:     detectLevel(message),
				Message:   message,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})
	return entries
}

func detectLevel(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "panic"), strings.Contains(lower, "fatal"), strings.Contains(lower, "error"):
		return "error"
	case strings.Contains(lower, "warn"):
		return "warn"
	case strings.Contains(lower, "info"):
		return "info"
	default:
		return ""
	}
}
