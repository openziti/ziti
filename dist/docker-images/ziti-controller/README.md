
# Run Ziti Controller in Docker

You can use this container image to run a Ziti Controller in a Docker container.

## Container Image

The `openziti/ziti-controller` image is thin and is based on the `openziti/ziti-cli` image, which only provides the
`ziti` CLI. The `ziti-controller` image adds an entrypoint that provides controller bootstrapping when
`ZITI_BOOTSTRAP=true` and uses the same defaults and options as the Linux package.

## Docker Compose

The included `compose.yml` demonstrates how to bootstrap a controller container.

### Example

At a minimum, you must set the address and password options in the parent env or set every recurrence in the compose file.

```text
# fetch the compose file for the ziti-router image
wget https://get.openziti.io/dist/docker-images/ziti-controller/compose.yml

ZITI_PWD="mypass" \
ZITI_CTRL_ADVERTISED_ADDRESS=ctrl.127.21.71.0.sslip.io \
    docker compose up
```

After a few seconds, `docker compose ps` will show a "healthy" status for the controller.

Then, you may log in to the controller using the `ziti` CLI.

```text
ziti edge login ctrl.127.21.71.0.sslip.io:1280 -u admin -p mypass
```
