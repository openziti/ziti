FROM ubuntu:rolling

RUN apt update && \
    apt install jq curl netcat-openbsd vim inetutils-ping net-tools lsof -y

RUN useradd -ms /bin/bash ziti && \
    mkdir /openziti && chown ziti:ziti /openziti && \
    mkdir /openziti/scripts && chown ziti:ziti /openziti/scripts && \
    mkdir /openziti/shared && chown ziti:ziti /openziti/shared && \
    mkdir /mutable-ziti && chown ziti:ziti /mutable-ziti

USER ziti
WORKDIR /openziti

ENV ZITI_HOME=/openziti
ENV ZITI_SHARED="${ZITI_HOME}/shared"
ENV ENV_FILE="${ZITI_HOME}/ziti.env"
ENV ZITI_NETWORK=ziti
# Don't put these paths on volume shared in docker-compose deployment (https://github.com/openziti/ziti/issues/912)
ENV MUTABLE_HOME=/mutable-ziti
ENV ZITI_BIN_DIR="${MUTABLE_HOME}/ziti-bin"
ENV ZITI_SCRIPTS="${MUTABLE_HOME}/scripts"

RUN mkdir -p "${ZITI_SCRIPTS}"

# copy the ziti binaries to a directory already on the path
COPY --chown=ziti ziti.ignore "${ZITI_BIN_DIR}"
COPY --chown=ziti ziti-cli-functions.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-controller.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-router.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-with-ziti-cli.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti access-control.sh "${ZITI_SCRIPTS}/"

RUN /bin/bash -c "source ${ZITI_SCRIPTS}/ziti-cli-functions.sh; ziti_createEnvFile; echo source ${ZITI_HOME}/ziti.env >> ~/.bashrc"
