# this Dockerfile builds docker.io/openziti/ziti-controller

ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"

# dependabot bumps this version based on release to Hub
# only amd64 is available because only static assets are copied, not executables
FROM --platform=linux/amd64 openziti/ziti-console-assets:3.4.5 AS ziti-console

FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

ARG CONTROLLER_PACKAGE=./dist/dist-packages/linux/openziti-controller
ARG DOCKER_BUILD_DIR=.

### Required Labels for OpenShift 
LABEL name="openziti/ziti-controller" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Controller" \
      description="Run the OpenZiti Controller" \
      org.opencontainers.image.description="Run the OpenZiti Controller" \
      org.opencontainers.image.source="https://github.com/openziti/ziti"

# set up image as root, then drop privs to ziggy
USER root

RUN INSTALL_PKGS="util-linux libbpf" && \
      microdnf -y update --setopt=install_weak_deps=0 --setopt=tsflags=nodocs && \
      microdnf -y install --setopt=install_weak_deps=0 --setopt=tsflags=nodocs ${INSTALL_PKGS}

# used by entrypoint.bash to source the bootstrapping script in this image
ENV ZITI_CTRL_BOOTSTRAP_BASH=/bootstrap.bash

COPY ${CONTROLLER_PACKAGE}/bootstrap.bash ${ZITI_CTRL_BOOTSTRAP_BASH}
COPY ${CONTROLLER_PACKAGE}/entrypoint.bash /

# copy the console SPA build files
COPY --from=ziti-console /usr/src/app/dist/app-ziti-console /ziti-console
ENV ZITI_CTRL_CONSOLE_LOCATION=/ziti-console

RUN mkdir -p /ziti-controller
RUN chown -R ziggy:ziggy /ziti-controller /ziti-console
WORKDIR /ziti-controller

# configure logging
ENV ZITI_TIME_FORMAT=utc

# globally toggle bootstrapping
ENV ZITI_BOOTSTRAP=true

# selectively toggle bootstrapping steps
ENV ZITI_BOOTSTRAP_PKI=true
ENV ZITI_BOOTSTRAP_CONFIG=true
ENV ZITI_BOOTSTRAP_DATABASE=true
ENV ZITI_BOOTSTRAP_CONSOLE=true

# defaults for bootstrapping PKI
ENV ZITI_PKI_ROOT=pki
ENV ZITI_CA_FILE=root
ENV ZITI_INTERMEDIATE_FILE=intermediate
ENV ZITI_SERVER_FILE=server
ENV ZITI_CLIENT_FILE=client
ENV ZITI_NETWORK_NAME=ctrl
ENV ZITI_AUTO_RENEW_CERTS=true

# defaults for bootstrapping config
ENV ZITI_CTRL_ADVERTISED_PORT=1280

# defaults for bootstrapping database
ENV ZITI_CTRL_DATABASE_FILE=bbolt.db
ENV ZITI_USER=admin

# run as ziggy (2171:2171) by default
USER ziggy
ENTRYPOINT [ "/entrypoint.bash" ]
