docker run --cap-add=NET_ADMIN --device /dev/net/tun --name ziti-tunneler-red --user root --network docker_zitired -v docker_ziti-fs:/persistent --rm -it openziti/quickstart /bin/bash
