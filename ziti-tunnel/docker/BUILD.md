You'll need to pass in artifactory credentials (username and token) to retrieve the ziti
binaries. You'll also need to specify the ziti version:

    $ ziti_version="0.5.7-2546" \
    $ docker build \
        --build-arg ZITI_VERSION="${ziti_version}" \
        --build-arg ARTIFACTORY_USERNAME=shawn.carey \
        --build-arg ARTIFACTORY_TOKEN="AKCp5d..." \
        -t "netfoundry/ziti-tunnel:${ziti_version}" .

Ziti binaries are downloaded from https://netfoundry.artifactory.io/netfoundry/ziti-release
by default. You can pass the `ARTIFACTORY_BASE_URL` and `ARTIFACTORY_REPO` build arguments if your
ziti binaries are in a different artifactory server and/or repository.

You can also build for other architectures (e.g. linux/arm) with docker's `buildx`
sub-command. See https://docs.docker.com/buildx/working-with-buildx/ for specifics.

    $ ziti_version="0.5.7-2546"
    $ docker buildx create --use
    $ docker buildx build \
        --platform linux/amd64,linux/arm/v7 \
        --build-arg ZITI_VERSION="${ziti_version}" \
        --build-arg ARTIFACTORY_USERNAME=shawn.carey \
        --build-arg ARTIFACTORY_TOKEN="AKCp5d..." \
        -t "netfoundry/ziti-tunnel:${ziti_version}" . --push

Unfortunately `buildx` doesn't currently support building images directly into
the local docker cache. Although the `--load` and `--output=type=docker` options
exist, the underlying capability will be implemented in a future docker release
(see https://github.com/docker/buildx/issues/59). In the meantime, you'll need
to push your image builds (with the `--push` build option) and then pull them to
run the image locally when building with `buildx`.
