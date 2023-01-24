ARG ZITI_VERSION="latest"
# this builds docker.io/openziti/ziti-router
FROM docker.io/openziti/ziti-cli:${ZITI_VERSION}

# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

### Required OpenShift Labels 
LABEL name="openziti/ziti-tunnel" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti Go Tunneler" \
      description="Run the OpenZiti Go Tunneler"

### Add necessary Red Hat repos and packages: "mountpoint" used by entrypoint
USER root
RUN   INSTALL_PKGS="util-linux iptables" && \
      microdnf -y update --setopt=install_weak_deps=0 --setopt=tsflags=nodocs && \
      microdnf -y install --setopt=install_weak_deps=0 --setopt=tsflags=nodocs ${INSTALL_PKGS}

COPY ./entrypoint.sh /
RUN chmod +x /entrypoint.sh
ENTRYPOINT [ "/entrypoint.sh" ]
CMD [ "run" ]
