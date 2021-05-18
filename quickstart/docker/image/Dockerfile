FROM ubuntu:latest

RUN apt update && \
    apt install jq curl netcat-openbsd vim inetutils-ping net-tools -y

RUN useradd -ms /bin/bash ziti && \
    mkdir /openziti && chown ziti:ziti /openziti && \
    mkdir /openziti/scripts && chown ziti:ziti /openziti/scripts && \
    mkdir /openziti/shared && chown ziti:ziti /openziti/shared

USER ziti
WORKDIR /openziti

ENV ZITI_HOME=/openziti
ENV ZITI_BIN_DIR="${ZITI_HOME}/ziti-bin"

# copy the ziti binaries to a directory already on the path
#COPY --chown=ziti ziti.ignore /usr/local/bin/
COPY --chown=ziti ziti.ignore "${ZITI_BIN_DIR}"
COPY --chown=ziti *.sh ./scripts/

#ENV ZITI_HOME=/openziti
#ENV ZITI_BIN_DIR="${ZITI_HOME}/ziti-bin"
ENV ZITI_SHARED=${ZITI_HOME}/shared
ENV ZITI_SCRIPTS=${ZITI_HOME}/scripts
ENV ENV_FILE=${ZITI_HOME}/ziti.env
ENV ZITI_NETWORK=ziti

RUN source "ziti-cli-functions.sh" && \
    ziti_createEnvFile && \
    echo "source ${ZITI_HOME}/ziti.env" >> ~/.bashrc

