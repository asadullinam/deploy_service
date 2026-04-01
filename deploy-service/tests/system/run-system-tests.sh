#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f /shared/kubeconfig.yaml ]]; then
  echo "waiting for kubeconfig from k3s..."
fi

for _ in $(seq 1 120); do
  if [[ -s /shared/kubeconfig.yaml ]]; then
    break
  fi
  sleep 2
 done

if [[ ! -s /shared/kubeconfig.yaml ]]; then
  echo "kubeconfig was not produced by k3s" >&2
  exit 1
fi

tmp_kubeconfig="/tmp/system-kubeconfig.yaml"
sed -e 's#127.0.0.1#k3s#g' -e 's#localhost#k3s#g' /shared/kubeconfig.yaml > "$tmp_kubeconfig"
export KUBECONFIG="$tmp_kubeconfig"

for _ in $(seq 1 120); do
  if kubectl get nodes >/dev/null 2>&1; then
    break
  fi
  sleep 2
 done

kubectl get nodes -o wide
vcluster version

go test -tags=system ./internal/integration/kubernetes -count=1 -v
