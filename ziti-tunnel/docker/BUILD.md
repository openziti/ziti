The Dockerfile and scripts in this directory build a ziti-tunnel Docker image.

Ziti binaries are downloaded from https://netfoundry.artifactory.io/netfoundry/ziti-release
by default. The following build arguments are supported:

  | Build Argument       | Description                                                       |
  | -------------------- | ----------------------------------------------------------------- |
  | ZITI_VERSION         | The version of the ziti binaries to fetch from artifactory.       |
  | ARTIFACTORY_TOKEN    | An Artifactory token with read access to the artifact repository. This is not needed if using the default ARTIFACTORY_BASE_URL / ARTIFACTORY_REPO. |
  | ARTIFACTORY_BASE_URL | Defaults to "https://netfoundry.jfrog.io/netfoundry".             |
  | ARTIFACTORY_REPO     | Defaults to "ziti-release".                                       |

# Building multi-platform Images

Use this build method if you're pushing the ziti-tunnel image to a public image
registry.

This build method creates images for the amd64 and arm/v7 platforms, but it
requires experimental Docker features and may involve more setup than you're
willing to endure. See "Building for Development" if you just want to build the
ziti-tunnel image for use on your local workstation, and don't plan on pushing
the image to a public registry.

## Prerequisites

1. Enable Docker Experimental Features

   See https://docs.docker.com/engine/reference/commandline/cli/#experimental-features

2. Install & Enable qemu Emulation for Arm (Docker CE / Linux only)

   This is taken care of by Docker Desktop if you're building on macOS or Windows,
   but you'll need to install qemu emulation support and register Arm binaries to
   run on your (presumably) x86_64 build host if you are running Docker CE on Linux:

       $ sudo dnf install -y qemu-system-arm
       $ docker run --rm --privileged docker/binfmt:66f9012c56a8316f9244ffd7622d7c21c1f6f28d

   Verify that the Arm qemu handler is registered:

       $ cat /proc/sys/fs/binfmt_misc/qemu-arm
       enabled
       interpreter /usr/bin/qemu-arm
       ...

   Ensure that the first line of the file is "enabled".

3. Create a Builder Instance

       $ docker buildx create --use

## Building

Run `docker buildx` like this:

    $ ziti_version="0.15.3"
    $ docker buildx build \
        --platform linux/amd64,linux/arm/v7,linux/aarch64 \
        --build-arg ZITI_VERSION="${ziti_version}" \
        -t "netfoundry/ziti-tunnel:${ziti_version}" .

Notes:

- You'll need to append "--push" to this command, and then subsequently pull the
  image to be able to use the image locally.

  Unfortunately `buildx` doesn't currently support building images directly into
  the local docker cache. Although the `--load` and `--output=type=docker` options
  exist, the underlying capability will be implemented in a future docker release
  (see https://github.com/docker/buildx/issues/59). In the meantime, you'll need
  to push your image builds (with the `--push` build option) and then pull them to
  run the image locally when building with `buildx`.

- The armv8 image uses armv7 (32-bit) ziti executables. The 32-bit compatibility
  libraries are installed in the image, but your Arm CPU must support 32-bit emulation.

## References:

- <https://docs.docker.com/buildx/working-with-buildx/>
- <https://www.docker.com/blog/multi-arch-images/>
- <https://community.arm.com/developer/tools-software/tools/b/tools-software-ides-blog/posts/getting-started-with-docker-for-arm-on-linux>
- <https://medium.com/@drpdishant/installing-docker-on-fedora-31-a073db823bb8>


# Building for Development

This build method produces an image for the CPU that is running the build host
(typically amd64), and places the resulting image into your local Docker image
cache.

    $ ziti_version="0.15.3" \
    $ docker build \
        --build-arg ZITI_VERSION="${ziti_version}" \
        -t "netfoundry/ziti-tunnel:${ziti_version}" .