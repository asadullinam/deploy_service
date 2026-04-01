package kubernetes

import (
	"context"
	"encoding/json"
	"time"
)

type KubectlProvisioner struct {
	kubectlPath                string
	vclusterPath               string
	contextName                string
	gatewayControllerNamespace string
	appsBaseDomain             string
	appsURLScheme              string
	appsURLPort                string
	monitoringNamespace        string
	prometheusBaseURL          string
	lokiBaseURL                string
	ingressNginxAutoInstall    bool
	ingressNginxManifestURL    string

	// Тестовые хуки
	runKubectlOverride               func(context.Context, []string, []byte) (string, error)
	runKubectlInVClusterOverride     func(context.Context, string, []string, []byte) (string, error)
	startVClusterPortForwardOverride func(context.Context, string, string, int) (func(), error)
	sleepOverride                    func(time.Duration)
}

type kubectlDeploymentList struct {
	Items []struct {
		Status struct {
			Replicas          int32 `json:"replicas"`
			ReadyReplicas     int32 `json:"readyReplicas"`
			AvailableReplicas int32 `json:"availableReplicas"`
		} `json:"status"`
	} `json:"items"`
}

type kubectlServiceList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Type  string `json:"type"`
			Ports []struct {
				Port int32 `json:"port"`
			} `json:"ports"`
		} `json:"spec"`
		Status struct {
			LoadBalancer struct {
				Ingress []struct {
					IP       string `json:"ip"`
					Hostname string `json:"hostname"`
				} `json:"ingress"`
			} `json:"loadBalancer"`
		} `json:"status"`
	} `json:"items"`
}

type kubectlPodList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Status struct {
			Phase             string `json:"phase"`
			ContainerStatuses []struct {
				Ready        bool  `json:"ready"`
				RestartCount int32 `json:"restartCount"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

type kubectlHTTPRouteList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Hostnames []string `json:"hostnames"`
		} `json:"spec"`
	} `json:"items"`
}

type kubectlIngressList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Rules []struct {
				Host    string          `json:"host"`
				HTTPRaw json.RawMessage `json:"http"`
			} `json:"rules"`
		} `json:"spec"`
	} `json:"items"`
}
