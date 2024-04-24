FROM ubuntu:18.04

ARG ARCH

RUN apt-get update \
    && apt-get -y install vim iptables build-essential wget git iputils-ping net-tools curl jq kafkacat apt-transport-https socat software-properties-common lsb-release gettext-base

ARG DOCKER_VER=19.03.8
ARG GO_VER=1.21.3

# install docker cli
RUN curl -4fsSLO https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VER}.tgz \
    && tar xzvf docker-${DOCKER_VER}.tgz --strip 1 -C /usr/bin docker/docker \
    && rm docker-${DOCKER_VER}.tgz

RUN curl https://dl.google.com/go/go${GO_VER}.linux-amd64.tar.gz | tar -xzf- -C /usr/local/

#RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \
#    && chmod 700 get_helm.sh \
#    && ./get_helm.sh

# Get specific versions of kubectl, minikube and helm that are known to work and work together.
# These steps get the CLI binaries and setup the config, including environment variables that the
# CLI needs.
WORKDIR /tmp
RUN curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/v1.10.0/bin/linux/amd64/kubectl
RUN chmod +x /tmp/kubectl
RUN mv /tmp/kubectl /usr/local/bin/
RUN curl -Lo minikube https://storage.googleapis.com/minikube-builds/2550/minikube-linux-amd64
RUN chmod +x /tmp/minikube
RUN mv /tmp/minikube /usr/local/bin/

RUN wget -q https://get.helm.sh/helm-v2.16.9-linux-amd64.tar.gz
RUN tar -xzvf /tmp/helm-v2.16.9-linux-amd64.tar.gz
RUN mv /tmp/linux-amd64/helm /usr/local/bin/

RUN mkdir -p /root/.kube
RUN touch /root/.kube/config

ENV MINIKUBE_WANTUPDATENOTIFICATION=false
ENV MINIKUBE_WANTREPORTERRORPROMPT=false
ENV MINIKUBE_HOME=/root
ENV CHANGE_MINIKUBE_NONE_USER=true
ENV KUBECONFIG=/root/.kube/config
# Done with Kubernetes (minikube) setup for this container.

RUN curl -4fsSL https://apt.releases.hashicorp.com/gpg | apt-key add - && \
    apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main" && \
    apt-get update && apt-get -y install vault ;

ENV GOROOT=/usr/local/go
ENV PATH="${PATH}:${GOROOT}/bin"

RUN adduser agbotuser --disabled-password --gecos "agbot user,1,2,3"

ENV HZN_ORG_ID="e2edev@somecomp.com"
ENV HZN_EXCHANGE_USER_AUTH="e2edev@somecomp.com/e2edevadmin:e2edevadminpw"

RUN mkdir -p /tmp/service_storage
WORKDIR /tmp

RUN alias dir='ls -la'
