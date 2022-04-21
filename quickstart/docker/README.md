Building and Deploying the Latest Quickstart Docker Image
==========================

Build Docker Image for Local Dev (probably for docker-compose testing)
------------------
1. change to this directory: `cd quickstart/docker`
2. run the script `./buildLocalDev.sh` which will create a `openziti/quickstart:dev` tag
3. update `.env` and change the value for `ZITI_VERSION` to `dev`
4. run `docker-compose` as normal

Build Docker Image For Publication
------------------
1. change to this directory: `cd quickstart/docker`
1. set an environment variable: `export ZITI_HOME=$(pwd)`
1. source the helper script: `source ../ziti-cli-functions.sh`
1. cleanup the binary directory if it exists: `rm -rf ./image/ziti.ignore`
1. issue this function to pull the latest ziti binaries: `getLatestZiti`
1. move the ziti binaries: `mv ziti-bin/ziti image/ziti.ignore/`
1. build the docker image: `docker build image -t openziti/quickstart`
1. exec into a container and make sure it's the version you expect: `docker run --rm -it openziti/quickstart ziti version`
1. cleanup: `rm ziti-*tar.gz; rm -rf ziti-bin`
2. 
Push Docker Image to dockerhub
------------------
1. `source ./image/ziti-cli-functions.sh`
1. just run `./pushLatestDocker.sh`