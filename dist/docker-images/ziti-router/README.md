
# Run Ziti Router in Docker

You can use this container image to run Ziti Router in a Docker container.

## Container Image

The `openziti/ziti-router` image is thin and is based on the `openziti/ziti-cli` image, which only provides the `ziti`
CLI. The `ziti-router` image simply adds the `ziti router` subcommand to prefix the args you supply.

## Docker Compose

The included `compose.yml` demonstrates how to bootstrap a router and assumes you have the enrollment token and know the
address of the controller, i.e., the `ctrl.endpoint` of the control plane listener provided by the OpenZiti controller.

### TPROXY Example

This demonstrates how to use the `openziti/ziti-router` image to run a Ziti Router in a Docker container to configure
the network namespace of another container to use the Ziti network.

```bash
# fetch the compose file for the ziti-router image
wget -O ./compose.router.yml https://get.openziti.io/dist/docker-images/ziti-router/compose.yml

# run the quickstart network in the background to provide the ctrl.endpoint at quickstart:1280
wget -O ./compose.quickstart.yml https://get.openziti.io/dock/all-in-one/compose.yml

# patch the Compose project to use the quickstart network and provide a web server to test the hello service
cat <<EOF >./compose.tproxy.yml
services:
  # add a hello web server to use for a Ziti service target
  hello:
    image: openziti/hello-world
    expose:
      - 8000
    networks:
      - quickstart

  # add a web client that waits for a healthy tproxy router
  tproxy-demo-client:
    image: busybox
    network_mode: service:ziti-router
    depends_on:
      ziti-router:
        condition: service_healthy
    command: wget --output-document=- http://hello.internal/

  # link the router to the quickstart network so it can reach the Ziti controller
  ziti-router:
    networks:
      - quickstart
EOF
export COMPOSE_FILE=compose.router.yml:compose.quickstart.yml:compose.tproxy.yml

# run the Ziti controller in the background with the all-in-one quickstart container
docker compose up quickstart-check

# start the hello web server listening on 8000
docker compose up hello --detach

# log in to the Ziti controller
ziti edge login 127.0.0.1:1280 -y -u admin -p admin

# create a Ziti service for the hello web server
ziti edge secure hello tcp:hello:8000 \
    --interceptAddress=hello.internal

# grant the quickstart router permission to bind (provide) the hello service
ziti edge update identity quickstart-router \
    --role-attributes=hello.servers

# create a second Ziti router to use as a tproxy client
ziti edge create edge-router "tproxy-router" \
   --jwt-output-file=./tproxy-router.jwt \
   --tunneler-enabled

# grant the tproxy client permission to dial (consume) the hello service
ziti edge update identity tproxy-router \
    --role-attributes=hello.clients

# simulate policies to check for authorization problems
ziti edge policy-advisor services -q

# run the demo client which triggers the run of the tproxy router because it is a dependency
ZITI_ROUTER_JWT="$(<./tproxyRouter.jwt)" \
ZITI_ROUTER_MODE=tproxy \
ZITI_CTRL_ADVERTISED_ADDRESS=quickstart \
ZITI_CTRL_ADVERTISED_PORT=1280 \
ZITI_ROUTER_PORT=3023 \
ZITI_ROUTER_ADVERTISED_ADDRESS=ziti-router \
    docker compose up tproxy-demo-client
```
