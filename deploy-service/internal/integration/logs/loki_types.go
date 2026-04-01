package logs

import (
	"net/http"
	"time"
)

type LokiReader struct {
	baseURL string
	client  *http.Client
}

type lokiQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string             `json:"resultType"`
		Result     []lokiStreamResult `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

type lokiStreamResult struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

const defaultTimeout = 15 * time.Second
