ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"
# this builds docker.io/openziti/ziti-router
FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

### Required OpenShift Labels
LABEL name="openziti/ziti-router" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Router" \
      description="Run the OpenZiti Router"

# set up image as root, then drop privs to ziggy
USER root

RUN INSTALL_PKGS="util-linux iptables" && \
      microdnf -y update --setopt=install_weak_deps=0 --setopt=tsflags=nodocs && \
      microdnf -y install --setopt=install_weak_deps=0 --setopt=tsflags=nodocs ${INSTALL_PKGS}

# set a var for entrypoint.bash to find this script
ENV ZITI_ROUTER_BOOTSTRAP_BASH=/bootstrap.bash
ENV ZITI_ROUTER_TYPE=edge
ENV ZITI_ROUTER_ADVERTISED_PORT=3022
ENV ZITI_BOOTSTRAP_CONFIG=true
ENV ZITI_BOOTSTRAP_ENROLLMENT=true
ENV PFXLOG_NO_JSON=true
COPY ./dist/dist-packages/linux/openziti-router/bootstrap.bash ${ZITI_ROUTER_BOOTSTRAP_BASH}
COPY ./dist/dist-packages/linux/openziti-router/entrypoint.bash /

WORKDIR /ziti-router
RUN chown -R ziggy:ziggy /ziti-router

# run as ziggy (2171:2171) by default, override run-as user with root when DOCKER_ROUTER_MODE=tproxy
USER ziggy

ENTRYPOINT [ "/entrypoint.bash" ]
