Quick Start Up and Running
==========================

Build Docker Image
------------------

1. Copy Ziti binaries into ziti/quickstart/docker/image/ziti.ignore/

2. Build image:

    $ cd ziti/quickstart/docker
    $ docker-compose build

3. Start Ziti Environment

    $ docker-compose up -d

4. Clean Up
  
    $ docker-compose down; docker volume prune /y; docker volume rm docker_ziti-fs

