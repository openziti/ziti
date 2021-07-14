# Overview

This document covers how to deploy Ziti from the ground up on a single local machine. After following this guide, it
should be possible to extrapolate the setup to multiple machines. This setup includes both the Fabric and optional
Edge components:

- 1x Ziti Controller + Edge
- 1x Ziti Edge Router
- 1x Ziti Router
- 1x Ziti Tunneler
- 1x Ziti Service (a demo netcat service)

It will also include provisioning and running a Ziti Tunneler with a demo netcat service. The services will use Ziti
router egress.  The full connection will be:

    SDK -> Edge Router -> Router -> Service

It is also possible to host a service and thus avoid egress through a router:

    Client SDK -> Edge Router -> Routers (0...1) -> Edge Router -> Server SDK

This document will guide the former.

For binaries, this guide provides instructions to build them for your local environment. It is also possible to acquire
pre-built binaries from the [NetFoundry Artifactory repository](https://netfoundry.jfrog.io/netfoundry/webapp/#/artifacts/browse/tree/General/ziti-release/ziti-all)
which does require a NetFoundry login. Extract the contents an use the binaries for your platform - ensuring that they
are on your path.

## Build Requirements

If building your own binaries

- A Golang environment (1.12 or later)
- A NetFoundry email address tied to a Bitbucket account with SSH keys

## Quick steps to a Building Binaries

It is not possible to simply use `go get bitbucket.org/netfoundry/ziti` as `go get` does not support specifying an
identity file (i.e. SSH keys). Instead the repo must first be retrieved by `git clone`.

### Linux/MacOS

    repo_dir=~/repos/nf
    mkdir ${repo_dir}
    cd ${repo_dir}
    git clone git@bitbucket.org:netfoundry/ziti.git
    cd ziti
    export GOPATH=$(pwd)/build
    go install ./fabric/... ./edge/... ./cli/...

### Windows

    set repo_dir=%userprofile%\repos\nf
    mkdir %repo_dir%
    cd /d %repo_dir%
    git clone git@bitbucket.org:netfoundry/ziti.git
    cd /d %repo_dir%\ziti
    set GOPATH=%cd%\build
    go install ./fabric/... ./edge/... ./cli/...

After you have cloned your repository and ensured it can build, change to $GOPATH or
%GOPATH% and execute `bin/ziti`. If everything is working properly you should be greeted
with the ziti command line help.

## Setting up your first environment

Now that you have ziti cloned and compiling, the next step is to get your very first
environment running.  These steps will bring you through setting up your first environment
with ziti. We will:

- [Requirements](#Requirements)
- [Establish Environment Variables](#Establish-Environment-Variables)
- [Create A Certificate Authority](#Create A Certificate Authority)
- [Configure & Run A Ziti Controller](#Configure-&-Run-A-Ziti-Controller)
- [Configure & Run A Ziti Router](#Configure-&-Run-A-Ziti-Router)
- [Configure & Run A Ziti Edge Router](#Configure-&-Run-A-Ziti-Edge-Router)
- [Configure A Service & Ziti Tunneler](#Configure-A-Service-&-Ziti-Tunneler)
- [Configuring a hosted service](#Configuring-A-Hosted-Service)

## Requirements

### Required Tooling

- A bash shell
- nc (netcat)

### A note on Windows

These commands all require a running bash shell. Windows users this means you'll need to use
WSL, [cygwin](https://www.cygwin.com/), a Linux virtual machine, or some other environment that supports a bash compliant
shell. The easiest thing might just be to use the shell that comes with [git bashfor windows](https://gitforwindows.org/).
WSL is maturing more and more: [Mintty and WSL](https://github.com/mintty/wsltty).

Also note that commands for `ziti`, `ziti-fabric`, `ziti-controller`, and `ziti-router` may need to have the `.exe`
suffix added into the command provided in this document.

### Hosts File

This document assumes that the following hostnames will resolve on your local system:

- up-and-running-ctrl.ziti.netfoundry.io
- up-and-running-er01.ziti.netfoundry.io
- up-and-running-r01.ziti.netfoundry.io

This means that temporarily adding these to your system hosts file (outlined below) is required for this guide.

- Windows: `windows: %windir%\system32\drivers\etc\hosts`
- Linux: `/etc/hosts`

    127.0.0.1   up-and-running-ctrl.ziti.netfoundry.io
    127.0.0.1   up-and-running-er01.ziti.netfoundry.io
    127.0.0.1   up-and-running-r01.ziti.netfoundry.io

If you are deploying on multiple machines with their own resolvable hostnames, this is not required. However, it will
be necessary to alter the Environment Variable DNS San values to match your environment. Also, all generated Ziti
Identities (server, client, and private keys), configuration files, and binaries will need to be copied to the correct
hosts.

## Establish Environment Variables

The environment variables ZITI_HOME and ZITI_NETWORK will be used to boostrap an environment configuration file and
directory structure to hold the various certificate, private keys, configuration files, and enrollment tokens that will
be generated in this guide. This will help keep them separated on your file system for easy deletion later.

After these are defined an environment folder and 'env' file will be generated that can easily reset these values.

1. Define the boot strap environment variables

        export ZITI_NETWORK=up-and-running
        export ZITI_HOME=~/.ziti

    For Windows users: if a bash shell is being used some absolute paths may not work (i.e. /mnt/c/myziti). Using paths
that start with `~` are generally safer. Using `.` will make configuration files and `env` files sensitive to the
current working directory, but should work as long as the correct current working directory is maintained.

1. Generate the base directory structure and `env` file

        mkdir -p $ZITI_HOME
        
        echo -e "export ZITI_HOME=${ZITI_HOME}\n\
        export ZITI_NETWORK=${ZITI_NETWORK}\n\
        export ZITI_ID=\"\${ZITI_HOME}/identities.yml\"\n\
        export ZITI_CA_NAME=\"\${ZITI_NETWORK}\"\n\
        export ZITI_PKI=\"\${ZITI_HOME}/pki\"\n\
        export ZITI_CA_FILE=\"\${ZITI_PKI}/\${ZITI_CA_NAME}/certs/\${ZITI_CA_NAME}.cert\"\n\
        export ZITI_CTRL_HOSTNAME=\"\${ZITI_NETWORK}-ctrl.ziti.netfoundry.io\"\n\
        export ZITI_ER01_HOSTNAME=\"\${ZITI_NETWORK}-er01.ziti.netfoundry.io\"\n\
        export ZITI_R01_HOSTNAME=\"\${ZITI_NETWORK}-r01.ziti.netfoundry.io\"\n\
        export ZITI_EDGE_API_PORT=1280\n\
        export ZITI_EDGE_API_HOSTNAME=\"\${ZITI_CTRL_HOSTNAME}:\${ZITI_EDGE_API_PORT}\"\n\
        mkdir -p \$ZITI_HOME/db
        mkdir -p \$ZITI_PKI
        " > ${ZITI_HOME}/env

    This file generated by the above snippet can be sourced any time to switch between working with multiple environments
    generated by this document or when starting new terminal windows.

    Source the newly created `env` file: `source ${ZITI_HOME}/env` or if the defaults were used `source .ziti/env`

    - `ZITI_HOME` - The root directory that should be consider the "HOME" directory (defaults to ~ if not set)
    - `ZITI_NETWORK` - The name of the network being worked with, this value is used as the basis for other file names. Should not contain spaces
    - `ZITI_PKI` - The location of the PKI used by `ziti pki` via the `--pki-root` option
    - `ZITI_CA_NAME` - The name of the signing CA that will be used for certificate generation and controller configuration. Defaults to the name of the network.
    - `ZITI_CA_FILE` - Location of the CA signing cert for ease of use using `ziti` CLI commands
    - `ZITI_CTRL_HOSTNAME` - DNS SANs for controller certificate generation, should match hosts file changes
    - `ZITI_ER01_HOSTNAME` - DNS SANs for edge router configuration, should match hosts file changes
    - `ZITI_R01_HOSTNAME` - DNS SANs for router certificate generation, should match hosts file changes
    - `ZITI_EDGE_API_PORT` - The port the Edge API will be available on
    - `ZITI_EDGE_API_HOSTNAME` - The hostname/port combination for the Ziti Edge API, used with `ziti edge controller` commands

## Create A Certificate Authority

All communication between Ziti components is secured using mutual TLS. In order to get a ziti environment running a
certificate authority is required. This document guides the reader through creating a Root Certificate Authority
(Root CA). It does not cover setups that should utilize Intermediate Certificate Authorities for production environments.

The `ziti` CLI will be significantly streamline and reduce the prerequisite knowledge required to generate
a Root CA and signed certificates for client and server usage. Specifically the `ziti pki` command do most of the heavy
lifting.

1. Ensure that the `env` file is sourced if you are using a new terminal.

1. Create the CA

        ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${ZITI_CA_NAME}"

    Additional certificate common name and subject fields can be set as needed. Run `ziti pki create ca -h` for options.

1. (Optional) Verify the output

        ls -l $ZITI_PKI/$ZITI_CA_NAME

    Example:

            total 0
            drwxr-xr-x 1 cd cd 512 Jan 28 16:39 certs
            drwx------ 1 cd cd 512 Jan 28 16:39 crls
            drwx------ 1 cd cd 512 Jan 28 16:39 keys
            -rw-r--r-- 1 cd cd   3 Jan 28 16:39 crlnumber
            -rw-r--r-- 1 cd cd 162 Jan 28 16:39 index.txt
            -rw-r--r-- 1 cd cd  20 Jan 28 16:39 index.txt.attr
            -rw-r--r-- 1 cd cd   3 Jan 28 16:39 serial

## Configure-&-Run-A-Ziti-Controller

The following steps outline how to configure and start the Ziti Controller.

1. Ensure that the `env` file is sourced if you are using a new terminal.

1. Generate the controller Ziti Identity (server, client, and private key)

    Server

        ziti pki create server --pki-root=$ZITI_PKI --ca-name $ZITI_CA_NAME \
        --server-file "${ZITI_NETWORK}-ctrl-server" \
        --dns "${ZITI_CTRL_HOSTNAME}" --ip "127.0.0.1" \
        --server-name "${ZITI_NETWORK} Controller"

    Client

        ziti pki create client --pki-root=$ZITI_PKI --ca-name ${ZITI_CA_NAME} \
        --client-file "${ZITI_NETWORK}-ctrl-client" \
        --key-file "${ZITI_NETWORK}-ctrl-server" \
        --client-name "${ZITI_NETWORK} Controller"

    Note: The client cert uses the controller server key as specified by `--key-file`.

    Additional certificate common name and subject fields can be set as needed. Run `ziti pki create server -h` or
     `ziti pki create client -h` for options.

    The Ziti Identity files are output in the following locations

        $ZITI_PKI/$ZITI_CA_NAME/certs/${ZITI_NETWORK}-ctrl-server.cert
        $ZITI_PKI/$ZITI_CA_NAME/certs/${ZITI_NETWORK}-ctrl-client.cert
        $ZITI_PKI/$ZITI_CA_NAME/keys/${ZITI_NETWORK}-ctrl-server.key

1. Generate the controller configuration file

        echo "
        v: 3
        db:                     $ZITI_HOME/db/ctrl.db
        
        identity:
          cert:                 ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-ctrl-client.cert
          server_cert:          ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-ctrl-server.cert
          key:                  ${ZITI_PKI}/${ZITI_CA_NAME}/keys/${ZITI_NETWORK}-ctrl-server.key
          ca:                   ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}.cert
        
        ctrl:
          listener:             tls:127.0.0.1:6262
        
        mgmt:
          listener:             tls:127.0.0.1:10000    
        edge:
          api:
            listener:  0.0.0.0:1280
            advertise: ${ZITI_EDGE_API_HOSTNAME}
            sessionTimeoutMinutes: 30
          enrollment:
            signingCert:
              cert: ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}.cert 
              key:  ${ZITI_PKI}/${ZITI_CA_NAME}/keys/${ZITI_NETWORK}.key
            edgeIdentity:
              durationMinutes: 5
            edgeRouter:
              durationMinutes: 5
         " > $ZITI_HOME/controller.yaml

1. Initialize the database if this is the first time you are going to start the controller

        # if needed, initialize the controller
        ziti-controller edge init $ZITI_HOME/controller.yaml -u admin -p admin
        
        # run the controller
        ziti-controller run $ZITI_HOME/controller.yaml

    Note that running this command will commandeer your current terminal. Use `screen` or multiple terminals by using
    `source` on the `env` file.

## Configure & Run A Ziti Router

This section outlines the steps necessary to configure and connect a router to the Ziti Controller. The router will be
named `r01` which is represented in the Common Name field of the certificates for the router. Routers join a controller
and create a mesh to provide long-haul transport.

To enroll a Ziti Router, the command line utility `ziti-fabric` will be used that requires its own Ziti Identity in order
to connect to and control the fabric.

To configure more routers, repeat steps 3+ from below but be sure to

- alter the `--server-name`, `--client-name`, and `--dns` parameters in all commands
- update the hosts file with any new hostnames

1. The `ziti-fabric` command will be used to manage the fabric, a Ziti Identity must be generated to do that:

        ziti pki create client --pki-root="${ZITI_PKI}" --ca-name="${ZITI_CA_NAME}" \
        --client-file="${ZITI_NETWORK}-dotzeet" \
        --client-name "${ZITI_NETWORK} Management"

1. Generate an `identities.json` file that references the Ziti Identity:

       echo "
       ---
       default:
         caCert: ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}.cert
         cert: ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-dotzeet.cert
         key: ${ZITI_PKI}/${ZITI_CA_NAME}/keys/${ZITI_NETWORK}-dotzeet.key
         endpoint: tls:127.0.0.1:10000
       " > $ZITI_HOME/identities.yml

1. Verify the identity works:

        ziti-fabric list routers

   Note: If 127.0.0.1 was not included in the controller cert, use the `-e` flag to set the address to the controller
   (`ziti-fabric -e "tls:up-and-running-ctrl.ziti.netfoundry.io:10000" list routers` ) on this an all future
   `ziti-fabric` command.

1. Generate the Ziti Router identity

    Server

        ziti pki create server --pki-root=$ZITI_PKI --ca-name ${ZITI_CA_NAME} \
        --server-file "${ZITI_NETWORK}-r01-server" \
        --dns "${ZITI_R01_HOSTNAME}" --ip 127.0.0.1 \
        --server-name r01

    Client

        ziti pki create client --pki-root=$ZITI_PKI --ca-name ${ZITI_CA_NAME} \
        --client-file "${ZITI_NETWORK}-r01-client" \
        --key-file "${ZITI_NETWORK}-r01-server" \
        --client-name r01

1. Generate the router configuration file:

       echo "v: 2
       
       identity:
         cert:                 ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-r01-client.cert
         server_cert:          ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-r01-server.cert
         key:                  ${ZITI_PKI}/${ZITI_CA_NAME}/keys/${ZITI_NETWORK}-r01-server.key
         ca:                   ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-r01-server.chain.pem
       
       ctrl:
         endpoint:             tls:${ZITI_CTRL_HOSTNAME}:6262
       
       link:
         listener:             quic:127.0.0.1:6002
       
       listeners:
         - binding:            transport
           address:            tls:0.0.0.0:7002
       " > $ZITI_HOME/r01.yaml

1. Enroll the Ziti Router identity with the controller

        ziti-fabric create router "${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-r01-client.cert"

    Verify:

        ziti-fabric list routers

    Should show 1 router named `r01`.

1. Start the router:

        ziti-router run $ZITI_HOME/r01.yaml

    Note that running this command will commandeer your current terminal. Use `screen` or multiple terminals by using
    `source` on the `env` file.

## Configure & Run A Ziti Edge Router

A Ziti Edge Router allows the Ziti SDK to ingress to a Ziti overlay network. A Ziti Edge Router is a Ziti Router that has the `edge`
section of the configuration defined and has gone through the Edge Router Enrollment process instead of the Ziti Router
enrollment process. Ziti Edge Routers and Ziti Routers are run from the same binary `ziti-router`.

Ensure that the `env` file is sourced if you are using a new terminal.

1. Authenticate w/ the controller if not authenticated

        #update username / password if the default admin password has been updated
        ziti edge controller login ${ZITI_EDGE_API_HOSTNAME} -u admin -p admin -c  ${ZITI_CA_FILE}

1. Create a cluster

        ziti edge controller create cluster cluster01

1. Create a edge router and output the JWT to a file:

        ziti edge controller create edge-router er01 cluster01 -o $ZITI_HOME/er01.jwt

1. Create the edge router configuration file

       echo "
       v: 2
        
       identity:
         cert:                 ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-er01-client.cert
         server_cert:          ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-er01-server.cert
         key:                  ${ZITI_PKI}/${ZITI_CA_NAME}/keys/${ZITI_NETWORK}-er01-server.key
         ca:                   ${ZITI_PKI}/${ZITI_CA_NAME}/certs/${ZITI_NETWORK}-er01-server.chain.cert
       
       ctrl:
         endpoint:             tls:${ZITI_CTRL_HOSTNAME}:6262
       
       edge:         
         csr:
           country: US
           province: NC
           locality: Charlotte
           organization: NetFoundry
           organizationalUnit: Ziti
           sans:
             dns:
               - ${ZITI_NETWORK}-er01.ziti.netfoundry.io
             ip:
               - "127.0.0.1"

       dialers:
         - binding: udp
         - binding: transport
       
       listeners:
         - binding: edge
           address: tls:0.0.0.0:3022
           options:
             advertise: ${ZITI_NETWORK}-er01.ziti.netfoundry.io:3022
         - binding: transport
           address: tls:0.0.0.0:7099    
       " > $ZITI_HOME/er01.yaml

1. Complete the enrollment process with the controller

        ziti-router enroll $ZITI_HOME/er01.yaml --jwt $ZITI_HOME/er01.jwt

1. Start the edge router:

        ziti-router run $ZITI_HOME/er01.yaml

    Note that running this command will commandeer your current terminal. Use `screen` or multiple terminals by using
    `source` on the `env` file.

## Configure A Service & Ziti Tunneler

1. Authenticate w/ the controller if not authenticated

        #update username / password if the default admin password has been updated
        ziti edge controller login ${ZITI_EDGE_API_HOSTNAME} -u admin -p admin -c  ${ZITI_CA_FILE}

1. Create a service that will facilitate connecting to a local netcat server listening on port 7256 and that egresses
the Ziti Fabric on our "r01" router

        ziti edge controller create service netcat7256 localhost 7256 r01 tcp://localhost:7256

1. Create a Ziti Edge Identity for the Ziti Tunneler process

        ziti edge controller create identity device identity01 -o $ZITI_HOME/identity01.jwt

1. Create an AppWan to associate the Ziti Tunneler identity (identity01) to the service (netcat7256)

        ziti edge controller create app-wan appwan01 -s netcat7256 -i identity01

1. Enroll the Ziti Tunneler's identity

        ziti-enroller --jwt $ZITI_HOME/identity01.jwt -o $ZITI_HOME/identity01.json

1. Start the Ziti Tunneler in proxy mode

        ziti-tunnel proxy netcat7256:8145 -i $ZITI_HOME/identity01.json

1. Start the netcat server

        nc -k -n 127.0.0.1 -l 7256

1. Start the netcat client that will connect to the Ziti Tunnel proxy

        nc -v 127.0.0.1 8145

## Configuring A Hosted Service

Services can also be hosted by another SDK. For this to work a second Ziti Tunneler can be setup to act as the host. The
exact details of this are beyond this document but the high level steps are outlined below:

1. Create a new Ziti Identity via `ziti edge controller identity create` to allow another Ziti Tunneler to run
1. Enroll the Ziti Identitiy via `ziti-enroller`
1. Add the new Ziti Identity to the hosts of a new service or an existing service
1. Start the new Ziti Tunneler

Example Hosted Service

    ziti edge controller create service <service name> <dns host> <dns port> \
    --hosted
    --hosted-ids <someIdentityId>
    --tags tunneler.dial.addr=<address for tunneler to dial>

Example:

    ziti edge controller create service postgresql pg 5432 \-
    --hosted
    --hosted-ids 40c025cb-bc92-4a54-b55f-1429412f2644
    -tags tunneler.dial.addr=tcp:127.0.0.1:5432
