docker run --cap-add=NET_ADMIN --device /dev/net/tun --name ziti-tunneler-blue --user root --network docker_zitiblue -v docker_ziti-fs:/persistent --rm -it openziti/quickstart /bin/bash
