ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"
# this builds docker.io/openziti/ziti-controller
FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

ARG CONTROLLER_PACKAGE=./dist/dist-packages/linux/openziti-controller
ARG DOCKER_BUILD_DIR=.

# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

### Required OpenShift Labels 
LABEL name="openziti/ziti-controller" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Controller" \
      description="Run the OpenZiti Controller"

# set up image as root, then drop privs to ziggy
USER root

# selectively toggle bootstrapping steps
ENV ZITI_BOOTSTRAP_PKI=true
ENV ZITI_BOOTSTRAP_CONFIG=true
ENV ZITI_BOOTSTRAP_DATABASE=true

# defaults for bootstrapping PKI
ENV ZITI_PKI_ROOT=pki
ENV ZITI_CA_FILE=root
ENV ZITI_INTERMEDIATE_FILE=intermediate
ENV ZITI_SERVER_FILE=server
ENV ZITI_CLIENT_FILE=client

# defaults for bootstrapping config
ENV ZITI_CTRL_ADVERTISED_PORT=1280

# defaults for bootstrapping database
ENV ZITI_CTRL_DATABASE_FILE=bbolt.db
ENV ZITI_USER=admin

# emit human-friendly text logs
ENV PFXLOG_NO_JSON=true
# used by entrypoint.bash to source the bootstrapping script in this image
ENV ZITI_CTRL_BOOTSTRAP_BASH=/bootstrap.bash
# used by "ziti create config controller" as filename for the controller's identity files
ENV ZITI_NETWORK_NAME=ctrl
# used by ziti to format timestamps in output
ENV ZITI_TIME_FORMAT=utc

COPY ${CONTROLLER_PACKAGE}/bootstrap.bash ${ZITI_CTRL_BOOTSTRAP_BASH}
COPY ${CONTROLLER_PACKAGE}/entrypoint.bash /

# run as ziggy (2171:2171) by default
USER ziggy
ENTRYPOINT [ "/entrypoint.bash" ]
