ARG ZITI_CLI_TAG="latest-fips"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"

FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

ARG DOCKER_BUILD_DIR=./dist/docker-images/ziti-tunnel

LABEL name="openziti/ziti-tunnel" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Go Tunneler" \
      description="Run the OpenZiti Go Tunneler"

USER root

RUN INSTALL_PKGS="util-linux iptables" \
      && apt-get update \
      && apt-get install -y --no-install-recommends $INSTALL_PKGS \
      && rm -rf /var/lib/apt/lists/*

WORKDIR /netfoundry
COPY --chmod=0755 ${DOCKER_BUILD_DIR}/entrypoint.sh /
ENTRYPOINT [ "/entrypoint.sh" ]
CMD [ "run" ]
