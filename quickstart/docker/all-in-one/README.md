# All-in-one Docker quickstart

This Docker Compose project runs `ziti edge quickstart` in a container while persisting configs, PKI, database, etc. in
a Docker named volume. You may instead persist the state in a filesystem directory on the Docker host by setting env var
`ZITI_HOME` to the directory's path.

## Run Ziti

This is the primary use case for this project: running the `ziti edge quickstart` command in the official
`openziti/ziti-cli` container image.

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

3. Modify the configuration and bounce the container.

    Modify the configs in the `./quickstart/` sub-directory adjacent to the `compose.yml` file.

    ```bash
    docker compose up --force-recreate
    ```

4. Observe the logs

    ```bash
    docker compose logs --follow
    ```

5. Run the CLI inside the quickstart environment.

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

The default storage option is to store the database, etc., in a named volume managed by Docker. Alternatively, you can store things in a subdirectory on the Docker host by setting `ZITI_HOME`.

Destroy the old network so you can start over.

```bash
docker compose down --volumes
```

Run it again with a different storage location.

```bash
ZITI_HOME=./config docker compose up
```

