#!/bin/bash
:'
This script will stand up a testable standalone docker container that simulates running expressInstall locally.
Overall process
1. The test image will be built
2. Express Install will be run
3. The container will be stood up indefinitely for testing purposes
    * When finished use "docker stop quickstart-test && docker rm quickstart-test" to clean up the container
    * If preferred, you may remove the image as well using "docker rmi openziti/quickstart:test"

Prerequisites:
  * For some tests, you must have "ziti-edge-controller" added to your hosts file
'

# Build the docker image
docker build ../docker/image -f ../docker/image/TestDockerfile -t openziti/quickstart:test
docker rmi "$(docker images -f "dangling=true" -q)"

# Start up the docker container, this will run expressInstall
docker run --name quickstart-test -p 1280:1280 -p 3022:3022 -e ZITI_EDGE_CONTROLLER_RAWNAME=ziti-edge-controller -e ZITI_CONTROLLER_RAWNAME=ziti-edge-controller -e ZITI_EDGE_ROUTER_RAWNAME=ziti-edge-router openziti/quickstart:test

# Once the container has stopped, this indicates expressInstall has completed, start it back up for testing
docker start quickstart-test

echo -e "Express Install Docker environment ready for testing..."
