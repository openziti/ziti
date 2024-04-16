ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"
# this builds docker.io/openziti/ziti-router
FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

ARG ROUTER_PACKAGE=./dist/dist-packages/linux/openziti-router
ARG DOCKER_BUILD_DIR=.

### Required OpenShift Labels
LABEL name="openziti/ziti-router" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Router" \
      description="Run the OpenZiti Router"

# set up image as root, then drop privs to ziggy
USER root

RUN INSTALL_PKGS="util-linux iptables libbpf iproute" && \
      microdnf -y update --setopt=install_weak_deps=0 --setopt=tsflags=nodocs && \
      microdnf -y install --setopt=install_weak_deps=0 --setopt=tsflags=nodocs ${INSTALL_PKGS}

# set a var for entrypoint.bash to find this script
ENV ZITI_ROUTER_BOOTSTRAP_BASH=/bootstrap.bash

COPY ${ROUTER_PACKAGE}/bootstrap.bash ${ZITI_ROUTER_BOOTSTRAP_BASH}
COPY ${ROUTER_PACKAGE}/entrypoint.bash /

WORKDIR /ziti-router
RUN chown -R ziggy:ziggy /ziti-router

# run as ziggy (2171:2171) by default, override run-as user with root when DOCKER_ROUTER_MODE=tproxy
USER ziggy

# configure logging
ENV PFXLOG_NO_JSON=true
ENV ZITI_TIME_FORMAT=utc

# globally toggle bootstrapping; false preserves "thin" behavior for existing deployments like K8s
ENV ZITI_BOOTSTRAP=false

# selectively toggle bootstrapping steps
ENV ZITI_BOOTSTRAP_CONFIG=true
ENV ZITI_BOOTSTRAP_ENROLLMENT=true

# defaults for bootstrapping
ENV ZITI_ROUTER_NAME=router
ENV ZITI_ROUTER_TYPE=edge
ENV ZITI_ROUTER_PORT=3022
ENV ZITI_ROUTER_MODE=none

# renew router certs at startup with run --extend
ENV ZITI_AUTO_RENEW_CERTS=true


ENTRYPOINT [ "/entrypoint.bash" ]
