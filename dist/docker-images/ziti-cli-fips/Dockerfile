FROM ubuntu:22.04 AS openssl-fips

ARG TARGETARCH

ENV OPENSSL_BUILD_VERSION="3.1.2"
ENV OPENSSL_BUILD_TARBALL_SHA256="a0ce69b8b97ea6a35b96875235aa453b966ba3cba8af2de23657d8b6767d6539"
ENV OPENSSL_BUILD_CONFIGURE_ARGS="enable-fips -O2"

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
      curl \
      build-essential \
      && rm -rf /var/lib/apt/lists/*

RUN mkdir /usr/local/src/openssl
WORKDIR /usr/local/src/openssl

RUN curl -L -o openssl.tar.gz https://github.com/openssl/openssl/releases/download/openssl-${OPENSSL_BUILD_VERSION}/openssl-${OPENSSL_BUILD_VERSION}.tar.gz \
    && echo "${OPENSSL_BUILD_TARBALL_SHA256}  openssl.tar.gz" | sha256sum --quiet -c - \
    && tar --strip-components=1 -xzf openssl.tar.gz

# disable the aflag test because it doesn't work on qemu (aka cross compile, see https://github.com/openssl/openssl/pull/17945)
RUN PROC_COUNT=$(nproc); \
    JOBS=1; \
    if [ "$PROC_COUNT" -gt 1 ]; then JOBS=$(expr "$PROC_COUNT" - 1); fi; \
    if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
            ./Configure ${OPENSSL_BUILD_CONFIGURE_ARGS} && make -j"$JOBS" && make -j"$JOBS" test TESTS="-test_afalg"; \
      else \
            ./Configure ${OPENSSL_BUILD_CONFIGURE_ARGS} && make -j"$JOBS"; \
      fi

RUN make install_sw install_ssldirs install_fips

FROM mcr.microsoft.com/oss/go/microsoft/golang:1.24.1-1-bookworm AS msgo

ARG GO_LDFLAGS

RUN mkdir /usr/local/src/ziti
WORKDIR /usr/local/src/ziti

COPY go.mod go.sum ./
RUN   --mount=type=cache,target="/root/.cache/go-build" \
      --mount=type=cache,target=/go/pkg/mod \
      go mod download -x

COPY . .
RUN   --mount=type=cache,target="/root/.cache/go-build" \
      --mount=type=cache,target=/go/pkg/mod \
      CGO_ENABLED=1 \
      go build -tags 'goexperiment.opensslcrypto,requirefips' -trimpath \
      -ldflags "$GO_LDFLAGS" \
      -o /usr/local/bin ./...

# this Dockerfile builds docker.io/openziti/ziti-cli:{version}-fips

# get kubectl CLI from a source with Docker Content Trust (DCT)
# FIXME: require DCT at build time
FROM bitnami/kubectl:1.33 AS bitnami-kubectl

# FIXME: This repo requires terms acceptance and is only available on registry.redhat.io.
# FROM registry.access.redhat.com/openshift4/ose-cli AS openshift-cli

FROM ubuntu:22.04
# This build stage grabs artifacts that are copied into the final image.
# It uses the same base as the final image to maximize docker cache hits.

ARG ARTIFACTS_DIR=./release
ARG DOCKER_BUILD_DIR=./dist/docker-images/ziti-cli
ARG TARGETARCH

ARG ZUID=2171
ARG ZGID=2171

ARG HOME=/home/ziggy

LABEL name="openziti/ziti-cli" \
      maintainer="developers@openziti.org" \
      vendor="NetFoundry" \
      summary="Run the OpenZiti CLI" \
      description="Run the OpenZiti CLI"

USER root

# Set LD_LIBRARY_PATH to ensure custom OpenSSL is used, no need for ldconfig
ENV LD_LIBRARY_PATH="/usr/local/lib64:/usr/local/lib"
COPY --from=openssl-fips /usr/local/ /usr/local/
# generate openssl FIPS config and run self-tests (these are required to be compliant)
RUN   case "${TARGETARCH}" in \
            amd64) LIB_DIR=/usr/local/lib64 ;; \
            arm64) LIB_DIR=/usr/local/lib ;; \
            *) echo $TARGETARCH; exit 2 ;; \
      esac; \
      /usr/local/bin/openssl version -a \
      && /usr/local/bin/openssl fipsinstall -out /usr/local/ssl/fipsmodule.cnf -module "$LIB_DIR/ossl-modules/fips.so"

# always use FIPS (sets the default openssl config to use the FIPS provider), also the config dir for the self-built openssl is /usr/local/ssl and NOT /etc/ssl
COPY --chmod=0644 ${DOCKER_BUILD_DIR}/openssl.cnf /usr/local/ssl/openssl.cnf
ENV OPENSSL_CONF=/usr/local/ssl/openssl.cnf

### install packages
RUN   apt-get update \
      && DEBIAN_FRONTEND=noninteractive apt-get install \
            -y --no-install-recommends \
            bash-completion \
            curl \
            findutils \
            hostname \
            jq \
            less \
            python3 \
            python3-pip \
            tar \
            vim-tiny \
      && rm -rf /var/lib/apt/lists/*

### install Kubernetes CLI
COPY --from=bitnami-kubectl /opt/bitnami/kubectl/bin/kubectl /usr/local/bin/

RUN mkdir -p -m0755 /licenses
COPY ./LICENSE /licenses/apache.txt

RUN groupadd --gid ${ZGID} ziggy \
      && useradd --uid ${ZUID} --gid ${ZGID} --system --home-dir ${HOME} --shell /bin/bash ziggy \
      && mkdir -p ${HOME} \
      && chown -R ${ZUID}:${ZGID} ${HOME} \
      && chmod -R g+rwX ${HOME}

COPY --chmod=0755 --from=msgo /usr/local/bin/ziti /usr/local/bin/ziti

RUN mkdir -p /usr/local/bin /etc/bash_completion.d
RUN /usr/local/bin/ziti completion bash > /etc/bash_completion.d/ziti_cli

USER ziggy
ENV HOME=${HOME}
WORKDIR ${HOME}
COPY ${DOCKER_BUILD_DIR}/bashrc ${HOME}/.bashrc

ENTRYPOINT [ "ziti" ]
