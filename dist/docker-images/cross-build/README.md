
# Cross-build Container for Building the Linux Executables for this Ziti Project

Running this container produces three executables for each Ziti component, one for each platform architecture: amd64, arm, arm64. You may specify the target CPU architecture as a parameter to the `docker run` command.

## Local Development

This article supports local development by providing a local containerized method for cross-building Linux executables. Alternatively, you may push to your GitHub repo fork to run [the main GitHub Actions workflow](../../.github/workflows/main.yml) (CI), which will cross-build the binaries for macOS, Windows, and Linux. Please refer to [the main article about local development](../../doc/002-local-dev.md) for help getting up and running without Docker.

## Build the Container Image

You only need to build the container image once unless you change the Dockerfile or `./linux-build.sh` (the container's entrypoint).

```bash
# build a container image named "ziti-go-builder"
docker buildx build \
    --tag=ziti-go-builder \
    --build-arg uid=$UID \
    --build-arg gid=$GID \
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
    --name=ziti-go-builder \
    --volume=$PWD:/mnt \
    ziti-go-builder

# build only amd64 
docker run \
    --rm \
    --name=ziti-go-builder \
    --volume=$PWD:/mnt \
    ziti-go-builder \
        amd64
```

You will find the built artifacts in `./release`.

## Example Output

```bash
 ❯ docker buildx build \
    --tag=ziti-go-builder \
    --build-arg uid=$UID \
    --build-arg gid=$GID \
    --load \
    ./docker-images/cross-build/

[+] Building 55.3s (17/17) FINISHED                                                                                                                                   
 => [internal] load build definition from Dockerfile                                                                                                             0.0s
 => => transferring dockerfile: 1.08kB                                                                                                                           0.0s
 => [internal] load .dockerignore                                                                                                                                0.0s
 => => transferring context: 2B                                                                                                                                  0.0s
 => [internal] load metadata for docker.io/library/debian:bullseye-slim                                                                                          0.3s
 => [ 1/11] FROM docker.io/library/debian:bullseye-slim@sha256:d51d5c391d202d5e2e0294a9df6ff077ed40583b11831d347d418690da496c50                                  0.0s
 => => resolve docker.io/library/debian:bullseye-slim@sha256:d51d5c391d202d5e2e0294a9df6ff077ed40583b11831d347d418690da496c50                                    0.0s
 => [internal] load build context                                                                                                                                0.0s
 => => transferring context: 2.48kB                                                                                                                              0.0s
 => CACHED [ 2/11] RUN apt-get -y update                                                                                                                         0.0s
 => CACHED [ 3/11] RUN apt-get -y install gcc-arm-linux-gnueabihf g++-arm-linux-gnueabihf gcc-aarch64-linux-gnu                                                  0.0s
 => CACHED [ 4/11] RUN apt-get -y install wget build-essential                                                                                                   0.0s
 => [ 5/11] COPY ./linux-build.sh /usr/local/bin/                                                                                                                0.0s
 => [ 6/11] RUN wget -q https://go.dev/dl/go1.19.linux-amd64.tar.gz                                                                                             19.2s
 => [ 7/11] RUN tar -xzf go1.19.linux-amd64.tar.gz -C /usr/local/                                                                                                4.8s
 => [ 8/11] RUN mkdir /usr/share/go /usr/share/go_cache                                                                                                          0.2s
 => [ 9/11] RUN chown -R 1000:1000 /usr/share/go /usr/share/go_cache                                                                                             0.1s
 => [10/11] RUN go install github.com/mitchellh/gox@latest                                                                                                       1.5s
 => [11/11] WORKDIR /mnt                                                                                                                                         0.0s 
 => exporting to oci image format                                                                                                                               23.0s 
 => => exporting layers                                                                                                                                         17.7s 
 => => exporting manifest sha256:e105f30fba598a7ba1b734de806eafcb8d8fc1af170481b5bd632eb87456b7db                                                                0.0s
 => => exporting config sha256:36b3c039e48a40584c16792ba6e59777b4be90ac36867014361a9c35a3980e9f                                                                  0.0s
 => => sending tarball                                                                                                                                           4.8s
 => importing to docker                                                                                                                                          6.6s

 ❯ docker run \                                        
    --rm \
    --name=ziti-go-builder \
    --volume=$PWD:/mnt \
    ziti-go-builder           
Number of parallel builds: 4

-->       linux/arm: github.com/openziti/ziti/ziti
Number of parallel builds: 4

-->     linux/arm64: github.com/openziti/ziti/ziti
Number of parallel builds: 4

-->     linux/amd64: github.com/openziti/ziti/ziti
Building for amd64 finished with result 0
Building for arm64 finished with result 0
Building for arm finished with result 0

 ❯ ll release/*/linux/
release/amd64/linux/:
total 96M
-rwxr-xr-x 1 kbingham kbingham 96M Feb  6 11:20 ziti*

release/arm64/linux/:
total 101M
-rwxr-xr-x 1 kbingham kbingham 101M Feb  6 11:20 ziti*

release/arm/linux/:
total 95M
-rwxr-xr-x 1 kbingham kbingham 95M Feb  6 11:20 ziti*

 ❯ ./release/amd64/linux/ziti version
NAME             VERSION
ziti             v0.0.0
ziti-controller  v0.0.0
ziti-prox-c      not installed
ziti-router      v0.0.0
ziti-tunnel      v0.0.0
ziti-edge-tunnel v0.20.18-local
```
