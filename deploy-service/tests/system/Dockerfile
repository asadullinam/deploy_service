FROM golang:1.25-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git bash \
    && rm -rf /var/lib/apt/lists/*

RUN KUBECTL_VERSION="$(curl -fsSL https://dl.k8s.io/release/stable.txt)" \
    && curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" -o /usr/local/bin/kubectl \
    && chmod +x /usr/local/bin/kubectl

RUN curl -fsSL https://github.com/loft-sh/vcluster/releases/latest/download/vcluster-linux-amd64 -o /usr/local/bin/vcluster \
    && chmod +x /usr/local/bin/vcluster

WORKDIR /workspace/deploy-service
