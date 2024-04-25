
# Run Ziti Router in Docker

You can use this container image to run a Ziti Router in a Docker container.

## Container Image

The `openziti/ziti-router` image is thin and is based on the `openziti/ziti-cli` image, which only provides the `ziti`
CLI. This `ziti-router` image adds an entrypoint that provides router bootstrapping when `ZITI_BOOTSTRAP=true` and uses
the same defaults and options as the Linux package.

## Docker Compose

The included `compose.yml` demonstrates how to bootstrap a router and documents the most relevant environment variables
that influence bootstrapping.

### Standalone Example

```text
# create the router, saving the enrollment token to a file
ziti edge create edge-router "router1" \
   --jwt-output-file=./router1.jwt

# fetch the compose file for the ziti-router image
wget https://get.openziti.io/dist/docker-images/ziti-router/compose.yml

ZITI_ENROLL_TOKEN="$(<./router1.jwt)" \
ZITI_CTRL_ADVERTISED_ADDRESS=ctrl.127.21.71.0.sslip.io \
ZITI_CTRL_ADVERTISED_PORT=1280 \
ZITI_ROUTER_ADVERTISED_ADDRESS=router1.127.0.0.1.sslip.io \
ZITI_ROUTER_PORT=3022 \
    docker compose up
```

### Sidecar Example

You can use this image as a sidecar container that provides Ziti DNS and TPROXY interception to another container. This
contrived example provides a web server that listens on port 8000 and a client that waits for the webserver to be
available. The client container shares a network interface with the router container and waits for the router to be
healthy before running.

```text
# fetch the compose file for the ziti-router image
wget -O ./compose.router.yml https://get.openziti.io/dist/docker-images/ziti-router/compose.yml
# fetch the router tproxy compose overrides files
wget -O ./compose.tproxy.yml https://get.openziti.io/dist/docker-images/ziti-router/compose.override.yml
# fetch the all-in-one quickstart compose file
wget -O ./compose.quickstart.yml https://get.openziti.io/dock/all-in-one/compose.yml
```

Patch the Compose project to use the quickstart network and provide a web server to test the hello service.

```text
cat <<EOF >>./compose.tproxy.yml
    # link the router to the quickstart network so it can reach the Ziti controller
    networks:
      - quickstart

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
EOF
```

Your `compose.tproxy.yml` should look like this.

```text
services:
  ziti-router:
    dns:
      - 127.0.0.1
      - 1.1.1.1
    user: root
    cap_add:
      - NET_ADMIN
    networks:
      - quickstart

  hello:
    image: openziti/hello-world
    expose:
      - 8000
    networks:
      - quickstart

  tproxy-demo-client:
    image: busybox
    network_mode: service:ziti-router
    depends_on:
      ziti-router:
        condition: service_healthy
    command: wget --output-document=- http://hello.internal/
```

Define the Compose project files.

```text
export COMPOSE_FILE=compose.router.yml:compose.tproxy.yml:compose.quickstart.yml
```

Run the Ziti controller in the background with the all-in-one quickstart container.

```text
docker compose up quickstart-check
```

Start the hello web server listening on 8000.

```text
docker compose up hello --detach
```

Log in to the Ziti controller

```text
ziti edge login 127.0.0.1:1280 -y -u admin -p admin
```

Create a Ziti service for the hello web server.

```text
ziti edge create config "hello-intercept-config" intercept.v1 \
  '{"portRanges":[{"high":80,"low":80}],"addresses":["hello.internal"],"protocols":["tcp"]}'
ziti edge create config "hello-host-config" host.v1 \
  '{"address":"hello","port":8000,"forwardProtocol":true,"allowedProtocols":["tcp"]}'
ziti edge create service "hello" \
  --configs "hello-intercept-config,hello-host-config" \
  --role-attributes 'hello.services'
ziti edge create service-policy "hello-dial-policy" Dial \
  --semantic AnyOf \
  --service-roles '#hello.services' \
  --identity-roles '#hello.clients'
ziti edge create service-policy "hello-bind-policy" Bind \
  --semantic AnyOf \
  --service-roles '#hello.services' \
  --identity-roles '#hello.servers'
```

Grant the quickstart router permission to bind (provide) the hello service.

```text
ziti edge update identity quickstart-router \
    --role-attributes=hello.servers
```

Create a second Ziti router to use as a tproxy client.

```text
ziti edge create edge-router "tproxy-router" \
   --jwt-output-file=./tproxy-router.jwt \
   --tunneler-enabled
```

Grant the tproxy client permission to dial (consume) the hello service

```text
ziti edge update identity tproxy-router \
    --role-attributes=hello.clients
```

Simulate policies to check for authorization problems

```text
ziti edge policy-advisor services -q
```

Run the demo client which triggers the run of the tproxy router because it is a dependency.

```text
ZITI_ENROLL_TOKEN="$(<./tproxy-router.jwt)" \
ZITI_ROUTER_MODE=tproxy \
ZITI_CTRL_ADVERTISED_ADDRESS=quickstart \
ZITI_CTRL_ADVERTISED_PORT=1280 \
ZITI_ROUTER_PORT=3023 \
ZITI_ROUTER_ADVERTISED_ADDRESS=ziti-router \
    docker compose up tproxy-demo-client
```
