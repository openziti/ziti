# Overview

This local development README guides you to set up a running Ziti stack that is built from source in this repo without any downloads, containers, scripts, or magic.

## Minimum Go Version

You will need a version of [Go](https://go.dev/) that is as recent as the version used by this project. Find the current minimum version by running this command to inspect `go.mod`.

```bash
grep -Po '^go\s+\K\d+\.\d+$' go.mod
```

## Build and Install All Applications

This repo contains several Go applications. The easiest way to build and install all to `${GOPATH}/bin` is:

```bash
# build and install all apps
go install ./...
```

If you add `${GOPATH}/bin` to your executable search `${PATH}` then you may immediately run the newly-built binaries. For example,

```bash
$ ziti-controller version
v0.0.0
```

The remainder of this article will assume you have installed all apps to be available in your `PATH`.

## Build Applications Individually

### `ziti` CLI

```bash
# build the binary without installing in GOPATH
go build -o ./build/ziti ./ziti/cmd/ziti/
# execute the binary
./build/ziti version
```

### `ziti-controller`

```bash
# build the binary without installing in GOPATH
go build -o ./build/ziti-controller ./ziti-controller/
# execute the binary
./build/ziti-controller version
```

### `ziti-router`

```bash
# build the binary without installing in GOPATH
go build -o ./build/ziti-router ./ziti-router/
# execute the binary
./build/ziti-router version
```

## Run a Local Ziti Stack

Let's get a local Ziti stack up and running now that you have built and installed all the Ziti apps in this repo.

### Initialize the Go Workspace

This is optional. You may skip initializing a Go workspace if you use release builds from other Go modules that this project depends upon e.g. `openziti/edge`, `openziti/fabric`.

A Go workspace is the best way to use the checked out copy of other modules' source instead of downloading releases from GitHub.

For each Ziti module you wish to develop (modify) you will need to add it to your workspace.

For this example, we'll only add the `edge` module.

```bash
go work init
go work use .
go work use ../edge  # assumes openziti/edge is checked out in sibling dir "edge"
```

This produces a `go.work` file.

```bash
$ cat go.work
go 1.19

use (
    .
    ../edge
)
```

You will need to be aware of the checked out revision in each module because it is used to satisfy imports at a higher precedence than pinned versions in `go.mod`. That is, you may change the version of `edge` that is imported by this module at build time by checking out a different version in the adjacent `edge` repo.

### Initialize the Environment

The remainder of this local development tutorital will instruct you to run terminal commands with current working directory of the top-level of this checked-out repo. The generated configuration files will use filesystem paths that are relative to this directory.

```bash
# this ./db directory is ignored by Git and will house the tutorial files
mkdir -p ./db
```

### Initialize the Controller

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

### Run the Controller

Edge SDKs will connect to the running controller to authenticate and request a session.

```bash
ziti-controller run ./db/ctrl-config.yml
```

### Login to the Controller

This step will save a session token in the `ziti` CLI's configuration cache.

```bash
ziti edge login -u ADMIN_NAME -p ADMIN_PW
```

Subsequent `ziti` CLI commands will automatically re-use this session token. You'll need to perform this login step again when the token expires.

### Initialize an Edge Router

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

### Run the Edge Router

Edge SDKs will connect to the running edge router to connect to services.

```bash
ziti-router run ./db/router01-config.yml
```

## Further Exploration

At this point the controller should be running with one router. The Ziti Go SDK that is used by the apps in this repo can be found [in GitHub](https://github.com/openziti/sdk-golang).

<!-- TODO: add tutorial steps to use the running controller and router to create a functioning service with one of the Go SDK examples e.g. Reflect Server -->