package kubernetes

import (
	"net/http"
	"time"
)

type KubectlMetricsCollector struct {
	kubectlPath         string
	contextName         string
	prometheusBaseURL   string
	prometheusAuthToken string
	prometheusQueries   []string
	window              time.Duration
	client              *http.Client
}

type metricsPodList struct {
	Items []metricsPodItem `json:"items"`
}

type metricsPodItem struct {
	Spec struct {
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
	} `json:"spec"`
	Status struct {
		Phase     string `json:"phase"`
		StartTime string `json:"startTime"`
	} `json:"status"`
}

type metricsPVCList struct {
	Items []struct {
		Spec struct {
			Resources struct {
				Requests struct {
					Storage string `json:"storage"`
				} `json:"requests"`
			} `json:"resources"`
		} `json:"spec"`
		Status struct {
			Capacity struct {
				Storage string `json:"storage"`
			} `json:"capacity"`
		} `json:"status"`
	} `json:"items"`
}

type metricsPodUsageList struct {
	Items []struct {
		Containers []struct {
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"containers"`
	} `json:"items"`
}
