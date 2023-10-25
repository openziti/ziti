# this Dockerfile builds docker.io/openziti/ziti-cli

# get kubectl CLI from a source with Docker Content Trust (DCT)
# FIXME: require DCT at build time
FROM bitnami/kubectl as bitnami-kubectl

# FIXME: This repo requires terms acceptance and is only available on registry.redhat.io.
# FROM registry.access.redhat.com/openshift4/ose-cli as openshift-cli  

FROM registry.access.redhat.com/ubi8/ubi-minimal
# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

ARG ARTIFACTS_DIR=./release
ARG DOCKER_BUILD_DIR=.
# e.g. linux
ARG TARGETOS
# e.g. arm64
ARG TARGETARCH

ARG ZUID=2171
ARG ZGID=2171

### Required OpenShift Labels 
LABEL name="openziti/ziti-cli" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti CLI" \
      description="Run the OpenZiti CLI"

USER root

### install packages
RUN   INSTALL_PKGS="python3.11 python3.11-pip tar bash-completion vim-minimal less shadow-utils jq" && \
      microdnf -y update --setopt=install_weak_deps=0 --setopt=tsflags=nodocs && \
      microdnf -y install --setopt=install_weak_deps=0 --setopt=tsflags=nodocs ${INSTALL_PKGS}

### install OpenShift CLI (oc)
# FIXME: This repo requires terms acceptance and is only available on registry.redhat.io.
# COPY --from=openshift-cli  /path/to/oc /usr/local/bin/oc

### install Kubernetes CLI
COPY --from=bitnami-kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

### add license in the path prescribed by OpenShift
RUN mkdir -p -m0755 /licenses
COPY ./LICENSE /licenses/apache.txt

RUN groupadd --gid ${ZGID} ziggy \
      && adduser --uid ${ZUID} --gid ${ZGID} --system --home /home/ziggy --shell /bin/bash ziggy \
      && mkdir -p /home/ziggy \
      && chown -R ${ZUID}:${ZGID} /home/ziggy \
      && chmod -R g+rwX /home/ziggy
RUN mkdir -p /usr/local/bin
COPY ${ARTIFACTS_DIR}/${TARGETARCH}/${TARGETOS}/ziti /usr/local/bin/
RUN chmod 0755 /usr/local/bin/ziti

RUN /usr/local/bin/ziti completion bash > /etc/bash_completion.d/ziti_cli

COPY ${DOCKER_BUILD_DIR}/entrypoint.sh /
RUN chmod +x /entrypoint.sh
USER ziggy
COPY ${DOCKER_BUILD_DIR}/bashrc /home/ziggy/.bashrc
ENTRYPOINT [ "/entrypoint.sh" ]
CMD [ "ziti" ]
