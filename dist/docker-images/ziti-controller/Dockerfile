ARG ZITI_CLI_TAG="latest"
ARG ZITI_CLI_IMAGE="docker.io/openziti/ziti-cli"
# this builds docker.io/openziti/ziti-controller
FROM ${ZITI_CLI_IMAGE}:${ZITI_CLI_TAG}

# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

### Required OpenShift Labels 
LABEL name="openziti/ziti-controller" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Controller" \
      description="Run the OpenZiti Controller"

USER root
COPY ./entrypoint.sh /
RUN chmod +x /entrypoint.sh
USER ziggy
ENTRYPOINT [ "/entrypoint.sh" ]
CMD [ "run" ]
