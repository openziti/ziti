
# Run Ziti Controller in Docker

You can use this container image to run a Ziti Controller in a Docker container.

## Container Image

The `ziti-controller` provides controller bootstrapping when `ZITI_BOOTSTRAP=true` and uses the same defaults and environment variables as the Linux package.

## Docker Compose

The provided [`compose.yml`](https://get.openziti.io/dist/docker-images/ziti-controller/compose.yml) contains hints for bootstrapping a controller.

### Example

At a minimum, you must set the controller address and password, and the cluster trust domain and node name variables. You can export these from the parent environment or set them in an `.env` file.

```text
# fetch the compose file for the ziti-router image
wget https://get.openziti.io/dist/docker-images/ziti-controller/compose.yml

ZITI_PWD="mypass" \
ZITI_CTRL_ADVERTISED_ADDRESS=ctrl1.ziti.example.com \
ZITI_CLUSTER_TRUST_DOMAIN=ziti.example.com \
ZITI_CLUSTER_NODE_NAME=ctrl1 \
    docker compose up
```

After a few seconds, `docker compose ps` will show a "healthy" status for the controller.

Then, you may log in to the controller using the `ziti` CLI.

```text
ziti edge login ctrl1.ziti.example.com:1280 -u admin -p mypass
```
