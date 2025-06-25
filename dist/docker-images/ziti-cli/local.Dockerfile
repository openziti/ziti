ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"

FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

ARG TARGETARCH

COPY --chmod=0755 ./release/${TARGETARCH}/linux/ziti /usr/local/bin/ziti
