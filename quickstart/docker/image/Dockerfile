FROM ubuntu:latest

RUN apt update && \
    apt install jq dos2unix curl netcat-openbsd wait-for-it vim inetutils-ping net-tools -y

RUN useradd -ms /bin/bash ziti && \
    mkdir /openziti && \
    chown ziti:ziti /openziti

USER ziti
WORKDIR /home/ziti

# copy the ziti binaries to a directory already on the path
COPY --chown=ziti ziti.ignore /usr/local/bin/
COPY --chown=ziti *.sh ./

RUN export ZITI_HOME=/openziti && \
    export ZITI_CONTROLLER_RAWNAME=ziti-controller && \
    export ZITI_EDGE_CONTROLLER_RAWNAME=ziti-edge-controller && \
    export ZITI_EDGE_ROUTER_RAWNAME=ziti-edge-router && \
    export ZITI_EDGE_WSS_ROUTER_RAWNAME=ziti-edge-router-wss && \
    export ZITI_ROUTER_BR_RAWNAME=ziti-edge-router-br && \
    export ZITI_ROUTER_BLUE_RAWNAME=ziti-edge-router-blue && \
    export ZITI_ROUTER_RED_RAWNAME=ziti-edge-router-red && \
    export ZITI_ZAC_RAWNAME=ziti-zac && \
    /home/ziti/env.sh && \
    echo "source ~/ziti.env" >> ~/.bashrc && \
    echo "alias ziti-fabric='ziti-fabric -e \${fabric_controller_uri}'" >> ~/.bashrc

