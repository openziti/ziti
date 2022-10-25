# Overview

This local deployment README guides you to set up a running Ziti stack that is built from source in this repo without any downloads, containers, scripts, or magic. If you have not already built the apps in this repo you may go back to [the previous tutorial about getting set up for local development](./002-local-dev.md) for build instructions. The remainder of the tutorial will assume you have installed all the apps so they can be found in your shell's executable search `PATH`.

You will configure and run:

- `ziti-controller` with the provided demo PKI in `./etc`
- `ziti-router` as an edge router with a new identity

## A Note About Windows

These commands require a running BASH shell. Windows users will need to use WSL, [cygwin](https://www.cygwin.com/), a Linux virtual machine, or some other environment that supports BASH. The easiest thing might just be to use the shell that comes with [git bashfor windows](https://gitforwindows.org/). WSL is maturing more and more: [Mintty and WSL](https://github.com/mintty/wsltty).

Also note that commands for `ziti`, `ziti-controller`, and `ziti-router` may need to have the `.exe` suffix appended to the examples.

## Initialize the Environment

The remainder of this local development tutorital will instruct you to run terminal commands with current working directory of the top-level of this checked-out repo. The generated configuration files will use filesystem paths that are relative to this directory.

```bash
# this ./db directory is ignored by Git and will house the tutorial files
mkdir -p ./db
```

## Initialize the Controller

Before you can run the controller will initialize its configuration and database. We'll use the demo PKI that's checked in to this repo in `./etc/`.

```bash
ZITI_HOME=. \                              
ZITI_CTRL_LISTENER_ADDRESS=127.0.0.1 \
ZITI_CTRL_EDGE_LISTENER_HOST_PORT=127.0.0.1:1280 \
ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT=127.0.0.1:1280 \
ZITI_CTRL_IDENTITY_CERT=./etc/ca/intermediate/certs/ctrl-client.cert.pem \
ZITI_CTRL_IDENTITY_SERVER_CERT=./etc/ca/intermediate/certs/ctrl-server.cert.pem \
ZITI_CTRL_IDENTITY_KEY=./etc/ca/intermediate/private/ctrl.key.pem \
ZITI_CTRL_IDENTITY_CA=./etc/ca/intermediate/certs/ca-chain.cert.pem \
ZITI_SIGNING_CERT=./etc/ca/intermediate/certs/intermediate.cert.pem \
ZITI_SIGNING_KEY=./etc/ca/intermediate/private/intermediate.key.decrypted.pem \
    ziti create config controller \
        --output ./db/ctrl-config.yml
```

```bash
ziti-controller edge init ./db/ctrl-config.yml -u ADMIN_NAME -p ADMIN_PW
```

## Run the Controller

Edge SDKs will connect to the running controller to authenticate and request a session.

```bash
ziti-controller run ./db/ctrl-config.yml
```

## Login to the Controller

This step will save a session token in the `ziti` CLI's configuration cache.

```bash
ziti edge login -u ADMIN_NAME -p ADMIN_PW
```

Subsequent `ziti` CLI commands will automatically re-use this session token. You'll need to perform this login step again when the token expires.

## Initialize an Edge Router

Request an enrollment token from the controller for router01.

```bash
ziti edge create edge-router router01 \
    --jwt-output-file /tmp/router01.jwt \
    --tunneler-enabled
```

Generate a configuration file for router01.

```bash
ZITI_HOME=./db \
ZITI_CTRL_ADVERTISED_ADDRESS=127.0.0.1 \
ZITI_EDGE_ROUTER_RAWNAME=localhost \
    ziti create config router edge \
    --routerName router01 \
    --output ./db/router01-config.yml
```

Enroll router01 by presenting the token to the controller to receive a certificate in the filesystem location specified in the configuration file.

```bash
ziti-router enroll --jwt /tmp/router01.jwt ./db/router01-config.yml
```

## Run the Edge Router

Edge SDKs will connect to the running edge router to connect to services.

```bash
ziti-router run ./db/router01-config.yml
```

## Create Your First Service

A service is an entity that stores metadata about a server application. We'll attach two configs to the service:

The `ziti` CLI has an interactive tutorial to step you through creating your first service.

```bash
ziti edge tutorial first-service
```
