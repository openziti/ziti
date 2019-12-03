FROM ubuntu:latest

RUN apt update && \
    apt install jq dos2unix curl netcat-openbsd wait-for-it vim inetutils-ping net-tools -y

RUN useradd -ms /bin/bash ziti && \
    mkdir /netfoundry && \
    chown ziti:ziti /netfoundry

USER ziti
WORKDIR /home/ziti

# copy the ziti binaries to a directory already on the path
COPY --chown=ziti ziti.ignore /usr/local/bin/
COPY --chown=ziti *.sh ./

RUN echo "touched"
RUN /home/ziti/env.sh && \
    echo "source ~/ziti.env" >> ~/.bashrc && \
    echo "alias ziti-fabric='ziti-fabric -e \${fabric_controller_uri}'" >> ~/.bashrc
