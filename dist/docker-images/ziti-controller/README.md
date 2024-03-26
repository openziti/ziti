
# Run Ziti Controller in Docker

You can use this container image to run a Ziti Controller in a Docker container.

## Container Image

The `openziti/ziti-controller` image is thin and is based on the `openziti/ziti-cli` image, which only provides the
`ziti` CLI. The `ziti-controller` image uses the same bootstrapping defaults and option variables as the Linux package.

## Docker Compose

The included `compose.yml` demonstrates how to bootstrap a controller container.

### Example

At a minimum, you must set the address and password options in the parent env or set every recurrence in the compose file.

```bash
ZITI_PWD="mypass" \
ZITI_CTRL_ADVERTISED_ADDRESS=ctrl.127.0.0.1.sslip.io \
    docker compose up
```

After a few seconds, `docker compose ps` will show a "healthy" status for the controller.

Then, you may log in to the controller using the `ziti` CLI.

```bash
ziti edge login ctrl.127.0.0.1.sslip.io:1280 -u admin -p mypass
```

It's not always necessary to publish ports on every one of the Docker host's interfaces. You can instead publish the
controller port only on a particular interface address by setting `ZITI_INTERFACE`.

```bash
ZITI_PWD="mypass" \
ZITI_INTERFACE=127.21.71.0 \
ZITI_CTRL_ADVERTISED_ADDRESS=ctrl.127.21.71.0.sslip.io \
    docker compose up
```
