# All-in-one Docker quickstart

This Docker Compose project runs `ziti edge quickstart` in a container while persisting configs, PKI, database, etc. in
a Docker named volume.

## Run Ziti

This is the primary use case for this project: running the `ziti edge quickstart` command in the
`openziti/ziti-controller` container image.

1. You need only the `compose.yml` file in this directory to run your own Ziti network.

    ```bash
    wget https://get.openziti.io/dock/all-in-one/compose.yml
    ```

1. In this "all-in-one" sub-directory, pull the container images. This makes the latest official release image available
   locally.

    ```bash
    docker compose pull
    ```

2. Run it.

    ```bash
    docker compose up
    ```

3. Modify the configuration files mounted on `/home/ziggy/quickstart/` and bounce the container.

    ```bash
    docker compose up --force-recreate
    ```

4. Observe the logs

    ```bash
    docker compose logs --follow
    ```

5. Open the console in a browser: [localhost:1280/zac/](https://localhost:1280/zac/). If you override the default controller address then substitute the correct address in the URL like `https://${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}/zac/`.

6. Run the CLI inside the quickstart environment.

    ```bash
    docker compose exec quickstart ziti edge list identities
    ```

    ```buttonless title="Output"
    ╭────────────┬───────────────────┬─────────┬────────────┬─────────────╮
    │ ID         │ NAME              │ TYPE    │ ATTRIBUTES │ AUTH-POLICY │
    ├────────────┼───────────────────┼─────────┼────────────┼─────────────┤
    │ ZS1YAo4Gnj │ quickstart-router │ Router  │            │ Default     │
    │ cOmDAo4Gb  │ Default Admin     │ Default │            │ Default     │
    ╰────────────┴───────────────────┴─────────┴────────────┴─────────────╯
    results: 1-2 of 2
    ```

### Save config on the Docker Host

The default storage option is to store the database, etc., in a named volume managed by Docker. You may instead persist the state in a filesystem directory on the Docker host by setting an env var to the path relative to the compose.yml file on the Docker host, e.g., `ZITI_HOME=./config`. If you choose to mount a folder this way, you must also ensure that the UID assigned by variable ZIGGY_UID has read-write-list permissions for this folder on the Docker host.

Destroy the old network so you can start over.

```bash
docker compose down --volumes
```

Run it again with a different storage location.

```bash
ZITI_HOME=./config docker compose up
```

