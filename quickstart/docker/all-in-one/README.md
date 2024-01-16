# All-in-one Ziti Docker quickstart

This Docker Compose project runs `ziti edge quickstart` in a container while persisting configs, PKI, database, etc. in
a Docker named volume. You may instead persist the state in a filesystem directory on the Docker host by setting env var
`ZITI_HOME` to the directory's path.

## Run Ziti

This is the primary use case for this project: running the `ziti edge quickstart` command in the official
`openziti/ziti-cli` container image.

1. In this "all-in-one" sub-directory, pull the container images. This makes the latest official release image available
   locally.

    ```bash
    docker compose pull
    ```

2. Run the project.

    ```bash
    docker compose up
    ```

3. Modify configuration and bounce the container.

    If you set `ZITI_HOME=./persistent`, then you would modify the configs in `./persistent/` on the Docker host.
    Otherwise, you would modify the configs in the Docker named volume that's mounted on `/persistent`. For example,
    `docker compose exec quickstart bash` will get you a shell in the container where you can `cd /persistent` edit the
    configs with `vi`.

    ```bash
    docker compose up --force-recreate
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
the one you build locally with `go build`.

1. In the top-level directory of the `ziti` project, build the binary.

    ```bash
    go build -o ./build ./...
    ```

    The build command can also be run from this "all-in-one" sub-directory.

    ```bash
    go build -o ../../../build ../../../...
    ```

2. In the "all-in-one" sub-directory, with `Dockerfile` present:

    ```bash
    ZITI_QUICK_TAG=local docker compose up --build
    ```

    By adding this `--build` option to the `up` command, the container image is built from the Dockerfile with your
    locally built `ziti` binary instead of pulling the default `openziti/ziti-cli` container image from Docker Hub. In
    the `compose.yml`, the Docker build context is hard-coded to `../../../` (three levels up from this directory at the
    top level of a Git working copy of the source repo). Setting `ZITI_QUICK_TAG=local` tags the locally-built container
    image differently from the official release image's `:latest` tag so you can tell them apart.

### Troubleshooting

#### Changing File Locations

The Compose project file `compose.yml` and `Dockerfile` have file paths that represent the assumption they're placed in
a sub-directory three levels deep in a checked-out copy of the `openziti/ziti` source repository. This allows the
Dockerfile to copy the built binary from the top-level directory `./build`. You may set the environment variable
`ARTIFACTS_DIR` to a different path relative to the build context (top-level directory of the source repo) to change the
location where the container image build looks for the locally-built `ziti` binary.

#### Building `ziti` in Docker

If the binary you build on the Docker host doesn't run in the container due to an environment issue, such as a GLIBC
version mismatch, you have the option to build `ziti` in the container every time you run
`ZITI_QUICK_TAG=local docker compose up --build`.

Change `Dockerfile` like this, and run `ZITI_QUICK_TAG=local docker compose up --build` to build the
checked-out source tree and run the quickstart with the build.

```dockerfile
FROM golang:1.20-bookworm AS builder
ARG ARTIFACTS_DIR=./build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/${ARTIFACTS_DIR} ./...

FROM debian:bookworm-slim
COPY --from=builder /app/${ARTIFACTS_DIR} /usr/local/bin/

CMD ["ziti"]
```

#### Gotcha - Not Clobbering the Downloaded Container Image

With `docker compose up --build`, the downloaded container image specified in `image` is replaced locally with the one
built from the Dockerfile.  This clobbers any image you may have pulled from the registry, which can lead to confusion.
You can prevent this by setting environment variable like `ZITI_QUICK_TAG=local docker compose up --build` to avoid
clobbering the default `:latest` tag.

If you already clobbered `:latest` just run `ZITI_QUICK_TAG=latest docker compose pull` to refresh your local copy from
the registry.
