FROM ubuntu:latest

RUN apt update && \
    apt install jq dos2unix curl netcat-openbsd wait-for-it vim inetutils-ping net-tools -y

RUN useradd -ms /bin/bash ziti && \
    mkdir /openziti && \
    chown ziti:ziti /openziti

USER ziti
WORKDIR /openziti

# copy the ziti binaries to a directory already on the path
COPY --chown=ziti ziti.ignore /usr/local/bin/
COPY --chown=ziti *.sh ./

ENV ZITI_HOME=/openziti
ENV ENV_FILE=/openziti/ziti.env
ENV ZITI_NETWORK=ziti

RUN /openziti/env.sh && \
    echo "source /openziti/ziti.env" >> ~/.bashrc

