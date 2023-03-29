FROM ubuntu:rolling as fetch-ziti-bins

# optional arg to specify which version to fetch when local "ziti-bin" directory is not present
ARG ZITI_VERSION_OVERRIDE

COPY . /docker.build.context
RUN apt update && apt install jq curl -y && \
    # get local ziti build in "ziti-bin", if present.
    # otherwise fetch from releases (respecting ZITI_VERSION_OVERRIDE, if set).
    # put the result in /ziti-bin
    /bin/bash /docker.build.context/fetch-ziti-bins.sh /ziti-bin

FROM ubuntu:rolling

RUN apt update && \
    apt install jq curl netcat-openbsd vim inetutils-ping net-tools lsof -y

RUN useradd -ms /bin/bash ziti && \
    mkdir /persistent && chown ziti:ziti /persistent && \
    mkdir /persistent/scripts && chown ziti:ziti /persistent/scripts

USER ziti
WORKDIR /persistent

ENV ZITI_HOME=/persistent
ENV ZITI_SHARED="${ZITI_HOME}/shared"
ENV ENV_FILE="${ZITI_HOME}/ziti.env"
ENV ZITI_NETWORK=ziti
# Don't put these paths on shared volume in docker-compose deployment (https://github.com/openziti/ziti/issues/912)
ENV ZITI_BIN_DIR=/var/openziti/ziti-bin
ENV ZITI_BIN_ROOT="${ZITI_BIN_DIR}"
ENV ZITI_SCRIPTS=/var/openziti/scripts

# copy the ziti binaries to a directory already on the path
COPY --chown=ziti --from=fetch-ziti-bins /ziti-bin "${ZITI_BIN_DIR}"
COPY --chown=ziti ziti-cli-functions.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-controller.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-router.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-with-ziti-cli.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti access-control.sh "${ZITI_SCRIPTS}/"
COPY --chown=ziti run-router-external.sh "${ZITI_SCRIPTS}/"

RUN /bin/bash -c "source ${ZITI_SCRIPTS}/ziti-cli-functions.sh; ziti_createEnvFile; echo source ${ZITI_HOME}/ziti.env >> ~/.bashrc"
