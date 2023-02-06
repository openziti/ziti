
# Cross-build Container for Building the Linux Executables for this Ziti Project

Running this container produces three executables for each Ziti component, one for each platform architecture: amd64, arm, arm64. You may specify the target CPU architecture as a parameter to the `docker run` command.

## Local Development

This article supports local development by providing a local containerized method for cross-building Linux executables. Alternatively, you may push to your GitHub repo fork to run [the main GitHub Actions workflow](../../.github/workflows/main.yml) (CI), which will cross-build the binaries for macOS, Windows, and Linux. Please refer to [the main article about local development](../../doc/002-local-dev.md) for help getting up and running without Docker.

## Build the Container Image

You only need to build the container image once unless you change the Dockerfile or `./linux-build.sh` (the container's entrypoint).

```bash
# build a container image named "zitibuilder" with the same version of Go that's declared in go.mod
docker buildx build \
    --tag=zitibuilder \
    --build-arg uid=$UID \
    --build-arg gid=$GID \
    --build-arg golang_version=$(grep -Po '^go\s+\K\d+\.\d+(\.\d+)?$' go.mod) \
    --load \
    ./docker-images/cross-build/
```

## Run the Container to Build Executables for the Desired Architectures

Executing the following `docker run` command will:

1. Mount the top-level of this repo on the container's `/mnt`
2. Run `linux-build.sh ${@}` inside the container
3. Deposit built executables in `./release`

```bash
# build for all three architectures: amd64 arm arm64
docker run \
    --rm \
    --name=zitibuilder \
    --volume=$PWD:/mnt \
    zitibuilder

# build only amd64 
docker run \
    --rm \
    --name=zitibuilder \
    --volume=$PWD:/mnt \
    zitibuilder \
        amd64
```

You will find the built artifacts in `./release`.
