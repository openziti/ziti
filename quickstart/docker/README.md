# Building and Deploying the Latest Quickstart Docker Image
==========================

First, decide what you are trying to do. Are you trying to:

* build a `ziti` from source, bake it into a docker image, and run the docker container?
* dev on the scripts inside the docker container, or the Dockerfile/compose file?
* build an 'older' version of `ziti` into a docker image to run

## Build Docker Image for Local Dev
------------------
1. build the `ziti` CLI somewhere
1. change to this directory from checkout root: `cd quickstart/docker`
1. make a directory named `ziti-bin`: `mkdir ./ziti-bin` 
1. copy the `ziti` CLI to `./ziti-bin`: `cp /path/to/ziti ./ziti-bin`
1. run the script `./createLocalImage.sh` which will create a `openziti/quickstart:latest` tag
   using the `ziti` CLI located in `./ziti-bin`
   1. Optionally, you may provide an argument for the image tag. `./createLocalImage.sh <tagname>`

## Build Docker Image for Docker-related Changes
1. change to this directory from checkout root: `cd quickstart/docker`
1. run the script `./createLocalImage.sh` which will create a `openziti/quickstart:latest` tag
   using the latest `ziti` [release from GitHub](https://github.com/openziti/ziti/releases/latest)
   1. Optionally, you may provide an argument, e.g., `./createLocalImage.sh <tagname>`, to create a tag
      other than `latest`.

## Build Docker Image with Specific ziti Version
1. change to this directory from checkout root: `cd quickstart/docker`
1. set `ZITI_VERSION_OVERRIDE` to desired version: `export ZITI_VERSION_OVERRIDE=v0.28.4`
1. run the script `./createLocalImage.sh` which will create a `openziti/quickstart:latest` tag
   using the specified version of `ziti` [from GitHub](https://github.com/openziti/ziti/releases/tag/v0.28.4)
   1. Optionally, you may provide an argument, e.g., `./createLocalImage.sh <tagname>`, to create a tag
      other than `latest`.

## Build Docker Image For Publication
To publish the latest `ziti` CLI, use [the GitHub Action](https://github.com/openziti/ziti/actions/workflows/push-quickstart.yml).
It's preferable to use `main` as the branch to create the docker image from, but it's 
perfectly fine to use 'release-next' as the source if there are script-related changes that need
to be pushed out faster than waiting for a merge to main.
