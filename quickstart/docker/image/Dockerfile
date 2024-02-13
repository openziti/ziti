FROM ubuntu:rolling as fetch-ziti-bins

# optional arg to specify which version to fetch when local "ziti-bin" directory is not present
ARG ZITI_VERSION_OVERRIDE
ARG DEBIAN_FRONTEND=noninteractive

COPY . /docker.build.context
RUN apt-get update \
    && apt-get --yes install \
        jq \
        curl \
    && apt-get --yes autoremove \
    && apt-get clean autoclean \
    && rm -fr /var/lib/apt/lists/{apt,dpkg,cache,log} /tmp/* /var/tmp/*
    # to use a locally built `ziti` cli \
    #   create a ./ziti-bin directory adjacent to this Dockerfile \
    #   and include any desired binaries to include in the image
    # to use the latest released version of ziti \
    #   just build the Dockerfile \
    # to use a specific version of ziti, specify ZITI_VERSION_OVERRIDE then  \
    #   build the Dockerfile
RUN bash /docker.build.context/fetch-ziti-bins.sh /ziti-bin

FROM ubuntu:rolling
ARG RUNUSER=ziti
ARG RUNGROUP=ziti
ARG RUNUSERID=2171

RUN apt-get update \
    && apt-get --yes install \
        jq \
        curl \
        netcat-openbsd \
        vim \
        inetutils-ping \
        net-tools \
        lsof \
    && apt-get --yes autoremove \
    && apt-get clean autoclean \
    && rm -fr /var/lib/apt/lists/{apt,dpkg,cache,log} /tmp/* /var/tmp/*

RUN useradd -u $RUNUSERID -ms /bin/bash $RUNUSER && \
    mkdir /persistent && chown $RUNUSER:$RUNGROUP /persistent && \
    mkdir /persistent/scripts && chown $RUNUSER:$RUNGROUP /persistent/scripts

USER $RUNUSER

WORKDIR /persistent

ENV ZITI_HOME=/persistent
ENV ZITI_SHARED="${ZITI_HOME}/shared"
ENV ZITI_ENV_FILE="${ZITI_HOME}/ziti.env"
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

RUN /bin/bash -c " \
    source ${ZITI_SCRIPTS}/ziti-cli-functions.sh; \
    persistEnvironmentValues; \
    echo source ${ZITI_HOME}/ziti.env \
        >> ~/.bashrc \
    "
