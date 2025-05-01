
# Run Ziti Controller in Docker

You can use this container image to run a Ziti Controller in a Docker container.

## Container Image

The `ziti-controller` provides controller bootstrapping when `ZITI_BOOTSTRAP=true` and uses the same defaults and environment variables as the Linux package. At a minimum, you must set the controller address and password, and the cluster trust domain and node name variables. You can export these from the parent environment or set them in an `.env` file when using Docker Compose.

## Docker Compose

Place the provided [`compose.yml`](https://get.openziti.io/dist/docker-images/ziti-controller/compose.yml) in the same directory as your `.env` file.

```text title=".env"
ZITI_CTRL_ADVERTISED_ADDRESS="ctrl1.ziti.example.com"
ZITI_CLUSTER_TRUST_DOMAIN="ziti.example.com"
ZITI_CLUSTER_NODE_NAME="ctrl1"
ZITI_PWD="mypass"
```

Then, run:

```text
docker compose up
```

After a few seconds, `docker compose ps` will show a "healthy" status for the controller.

## Docker CLI

```text
docker run \
--name ziti-ctrl1 \
--publish 1280:1280 \
--env ZITI_CTRL_ADVERTISED_ADDRESS="ctrl1.ziti.example.com" \
--env ZITI_CLUSTER_TRUST_DOMAIN="ziti.example.com" \
--env ZITI_CLUSTER_NODE_NAME="ctrl1" \
--env ZITI_PWD="mypass" \
openziti/ziti-controller

```