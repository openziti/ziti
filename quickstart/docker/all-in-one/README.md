# minimal Ziti Docker quickstart

This Docker Compose project runs `ziti edge quickstart` in a container while persisting configs, PKI, database, etc. in the same directory `./persistent/`.

## Run Ziti

This is the primary use case for this project: running the `ziti edge quickstart` command in the official
`openziti/ziti-cli` container image.

1. In this "minimal" sub-directory, pull the container images.

    ```bash
    docker compose pull
    ```

2. Run the project.

    ```bash
    docker compose up --detach
    ```

3. Modify the state in `./persistent/`, and bounce the container.

    ```bash
    docker compose up --force-recreate --detach
    ```

4. Observe the logs

    ```bash
    docker compose logs quickstart --follow
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

## Develop Ziti

This is a secondary use case for this Docker Compose project that replaces the `ziti` binary in the container image with
the one you build locally with `go build` before running the `ziti edge quickstart` command.

1. In the top-level directory of the `ziti` project, build the binary.

    ```bash
    go build -o ./build ./...
    ```

    The build command can also be run from this "minimal" sub-directory.

    ```bash
    go build -o ../../../build ../../../...
    ```

2. In the "minimal" sub-directory, with `Dockerfile` present:

    ```bash
    docker compose up --detach --build
    ```

    By adding this `--build` option to the `up` command, the container image is built from the Dockerfile with your
    locally built `ziti` binary instead of pulling the default `openziti/ziti-cli` container image from Docker Hub. In
    the `compose.yml`, the Docker build context is defined with environment variable `ZITI_SRC_ROOT` which defaults to
    `../../../` (three levels up from this directory at the top level of a Git working copy of the source repo).

### Troubleshooting

#### Changing File Locations

The Compose project file `compose.yml` and `Dockerfile` have file paths that represent the assumption they're placed in
a sub-directory three levels deep in a checked-out copy of the `openziti/ziti` source repository. This allows the Dockerfile
to copy the built binary from the top-level directory `./build`. You can move these files outside the source tree if you
adjust the paths in both files.

#### Building `ziti` in the Dockerfile

If the binary you build on your host doesn't run in the container due to an environment issue, such as a GLIBC version
mismatch, you have the option to build `ziti` in the container every time you run `up --build`.

Change `Dockerfile` like this, and run `docker compose up --detach --build` to build the checked-out source tree and run
the quickstart with the build.

```dockerfile
FROM golang:1.20-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o ./build/ ./...

FROM debian:bookworm-slim
COPY --from=builder /app/build/ziti /usr/local/bin/

CMD ["ziti"]
```

#### Gotcha - Clobbering the Container Image

With `docker compose up --build`, the container image specified in `image` is replaced with the one built from the Dockerfile.
This clobbers any image you may have pulled from the registry unless you change the value of `image` or comment the line.

```yaml
    # commenting "image" avoids clobbering the image pulled from the registry
    # image: ${ZITI_QUICK_IMAGE:-docker.io/openziti/ziti-cli}:${ZITI_QUICK_TAG:-latest}
    build:
      context: ${ZITI_SRC_ROOT:-../../../} 
      dockerfile: ./quickstart/docker/minimal/Dockerfile
```

Next time you run `docker compose pull` the image from the registry will be refreshed in the local cache.
