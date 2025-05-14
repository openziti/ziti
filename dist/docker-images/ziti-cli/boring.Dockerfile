# this Dockerfile builds docker.io/openziti/ziti-controller

ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"

FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

ARG ARTIFACTS_DIR=./release
ARG DOCKER_BUILD_DIR=./dist/docker-images/ziti-cli
# e.g. linux
ARG TARGETOS
# e.g. arm64
ARG TARGETARCH

COPY --chmod=0755 ${ARTIFACTS_DIR}/${TARGETARCH}/${TARGETOS}/ziti-fips /usr/local/bin/ziti
