# this Dockerfile builds docker.io/openziti/ziti-cli

# get kubectl CLI from a source with Docker Content Trust (DCT)
# FIXME: require DCT at build time
FROM portainer/kubectl-shell:2.34.0 AS portainer-kubectl

FROM registry.access.redhat.com/ubi9/ubi-minimal
# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

ARG ARTIFACTS_DIR=./release
ARG DOCKER_BUILD_DIR=./dist/docker-images/ziti-cli
# e.g. arm64
ARG TARGETARCH
# e.g. linux
ARG TARGETOS

ARG ZUID=2171
ARG ZGID=2171

ARG HOME=/home/ziggy

### Required OpenShift Labels 
LABEL name="openziti/ziti-cli" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti CLI" \
      description="Run the OpenZiti CLI"

USER root

### install packages
RUN   INSTALL_PKGS="python3.11 python3.11-pip tar bash-completion vim-minimal less shadow-utils jq findutils hostname" && \
      microdnf -y update --setopt=install_weak_deps=0 --setopt=tsflags=nodocs && \
      microdnf -y install --setopt=install_weak_deps=0 --setopt=tsflags=nodocs ${INSTALL_PKGS} && \
      microdnf clean all

### install OpenShift CLI (oc)
# FIXME: This repo requires terms acceptance and is only available on registry.redhat.io.
# COPY --from=openshift-cli  /path/to/oc /usr/local/bin/oc

### install Kubernetes CLI
COPY --from=portainer-kubectl /usr/local/bin/kubectl /usr/local/bin/

### add license in the path prescribed by OpenShift
RUN mkdir -p -m0755 /licenses
COPY ./LICENSE /licenses/apache.txt

RUN groupadd --gid ${ZGID} ziggy \
      && adduser --uid ${ZUID} --gid ${ZGID} --system --home ${HOME} --shell /bin/bash ziggy \
      && mkdir -p ${HOME} \
      && chown -R ${ZUID}:${ZGID} ${HOME} \
      && chmod -R g+rwX ${HOME}
RUN mkdir -p /usr/local/bin
COPY --chmod=0755 ${ARTIFACTS_DIR}/${TARGETARCH}/${TARGETOS}/ziti /usr/local/bin/

RUN /usr/local/bin/ziti completion bash > /etc/bash_completion.d/ziti_cli

USER ziggy
ENV HOME=${HOME}
WORKDIR ${HOME}
COPY ${DOCKER_BUILD_DIR}/bashrc ${HOME}/.bashrc

ENTRYPOINT [ "ziti" ]
