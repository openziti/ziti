FROM debian:stable-slim as fetch-ziti-artifacts
# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

ARG ZITI_VERSION="0.15.3"

ARG ARTIFACTORY_TOKEN
ARG ARTIFACTORY_BASE_URL="https://netfoundry.jfrog.io/netfoundry"
# to fetch snapshots from the "feature-0.5" branch, set ZITI_REPO="ziti-snapshot/feature-0.5"
ARG ARTIFACTORY_REPO="ziti-release"

WORKDIR /tmp

RUN apt-get -q update && apt-get -q install -y --no-install-recommends curl ca-certificates
# workaround for `openssl rehash` not working on arm.
RUN /bin/bash -c "if ! compgen -G '/etc/ssl/certs/*.[0-9]' > /dev/null; then c_rehash /etc/ssl/certs; fi"

COPY fetch-ziti-bins.sh .
RUN bash ./fetch-ziti-bins.sh ziti-tunnel

################
#
#  Main Image
#
################

FROM debian:stable-slim

RUN mkdir -p /usr/local/bin /etc/ssl/certs
RUN apt-get -q update && apt-get -q install -y --no-install-recommends iptables
# Use 32-bit arm binaries on arm64 until we have 64-bit arm builds. This also affects fetch-ziti-bins.sh.
RUN if [ "$(uname -m)" = "aarch64" ]; then dpkg --add-architecture armhf; apt update; apt install -y libc6:armhf; fi
COPY --from=fetch-ziti-artifacts /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs
COPY --from=fetch-ziti-artifacts /tmp/ziti-tunnel /usr/local/bin
COPY ./docker-entrypoint.sh /
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT [ "/docker-entrypoint.sh" ]
CMD [ "run" ]