# Crossbuild Container for Building Linux Executables

Running this container produces three executables for each Ziti component, one for each platform architecture: amd64, arm, arm64. You may instead build for one or more of these by specifying the architecture as a parameter to the `docker run` command as shown in the example below.

## Local Development

This article supports local development by providing a local containerized method for crossbuilding Linux executables. Alternatively, you may push to your GitHub repo fork to run [the main GitHub Actions workflow](./.github/workflows/main.yaml) (CI) which will crossbuild the binaries for MacOS, Windows, and Linux. Please refer to [the main article about local development](./doc/002-local-dev.md) for help getting up and running without Docker.

## Build the Container Image

You only need to build the container image once unless you change the Dockerfile or `./linux-build.sh` (the container's entrypoint).

```bash
# find the latest Go distribution's semver
LATEST_GOLANG=$(curl -sSfL "https://go.dev/VERSION?m=text" | /bin/grep -Po '^go(\s+)?\K\d+\.\d+\.\d+$')
# build a container image named "zitibuilder" with the Dockerfile in the top-level of this repo
docker build \
    --tag=zitibuilder \
    --file=Dockerfile.linux-build \
    --build-arg latest_golang=${LATEST_GOLANG} \
    --build-arg uid=$UID \
    --build-arg gid=$GID .
```

## Run the Container to Build Executables for the Desired Architectures

Executing the following `docker run` command will:

1. Mount the top-level of this repo on the container's `/mnt`
2. Run `./linux-build.sh ${@}` inside the container
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
