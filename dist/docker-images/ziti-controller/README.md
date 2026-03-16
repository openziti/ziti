
# Run Ziti controller in Docker

You can use this container image to run a Ziti Controller in a Docker container.

## Container image

The `openziti/ziti-controller` image is thin and is based on the `openziti/ziti-cli` image, which only provides the
`ziti` CLI. The `ziti-controller` image adds an entrypoint that provides controller bootstrapping when
`ZITI_BOOTSTRAP=true` and uses the same defaults and options as the Linux package.

The controller always runs in clustered mode, even for a single-node deployment.

## Docker compose

The included `compose.yml` demonstrates how to bootstrap a controller container.

### Example

At a minimum, you must set the permanent external address and password in the parent environment or in an `.env` file.

```text
# fetch the compose file for the ziti-controller image
wget https://get.openziti.io/dist/docker-images/ziti-controller/compose.yml

ZITI_PWD="mypass" \
ZITI_CTRL_ADVERTISED_ADDRESS=ctrl.127.21.71.0.sslip.io \
    docker compose up
```

After a few seconds, `docker compose ps` will show a "healthy" status for the controller.

Then, log in to the controller using the `ziti` CLI.

```text
ziti edge login ctrl.127.21.71.0.sslip.io:1280 -u admin -p mypass
```

### Environment variables

These are the most relevant variables for bootstrapping. See `compose.yml` for the full list.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `ZITI_BOOTSTRAP_CLUSTER` | no | `true` | Set to `false` when joining an existing cluster |
| `ZITI_CTRL_ADVERTISED_ADDRESS` | yes | — | Permanent external address (DNS name) of this controller |
| `ZITI_CTRL_ADVERTISED_PORT` | no | 1280 | TCP port |
| `ZITI_CLUSTER_NODE_NAME` | no | `ziti-controller1` | Unique name for this cluster node |
| `ZITI_CLUSTER_TRUST_DOMAIN` | no | `ziti` | Trust domain shared by all cluster nodes (SPIFFE ID) |
| `ZITI_PWD` | yes | — | Password for the default admin user |

### Joining an existing cluster

To add a controller to an existing cluster, set `ZITI_BOOTSTRAP_CLUSTER=false` and provide the first node's root CA via a volume mount, and set `ZITI_CLUSTER_NODE_PKI` to that mountpoint. See the `compose.test.yml` for a working example.

### Certificate renewal

Leaf certificates (server and client) are valid for 365 days and are
automatically renewed at each container startup when
`ZITI_AUTO_RENEW_CERTS=true` (the default). Ensure the controller container
is restarted at least once a year — any routine restart (image upgrade, host
reboot, config change) is sufficient.
