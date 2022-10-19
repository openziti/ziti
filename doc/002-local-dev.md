# Overview

This local development README guides you to set up a running Ziti stack that is built from source in this repo without any downloads, containers, scripts, or magic. If you are looking for more automation and less do-it-yourself then check out [the quickstarts](https://openziti.github.io/ziti/quickstarts/quickstart-overview.html).

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

## Crossbuilds

When you push to your repo fork then GitHub Actions will automatically crossbuild for several OSs and CPU architectures. You'll then be able to download the built artifacts from the GitHub UI. The easiest way to crossbuild the Linux exectuables locally is to build and run the crossbuild container. Please refer to [the crossbuild container README](../Dockerfile.linux-build.README) for those steps. For hints on crossbuilding on MacOS and Windows see [the main GitHub Actions workflow](../.github/workflows/main.yml) which defines the steps that are run when you push to GitHub.

## Run a Local Ziti Stack

Let's get a local Ziti stack up and running now that you have built and installed all the Ziti apps in this repo.

### Initialize the Environment

You'll need to define two environment variables that are employed by the included controller and router configuration YAML files. The must be exported to child processes of the current shell.

1. `ZITI_SOURCE`: the parent directory of this Git repo's checked-out copy

    ```bash
    # assuming PWD is your checked-out copy of this repo
    export ZITI_SOURCE=..
    ```

1. `ZITI_DATA`: the directory where the controller's database and the router's identity will be stored

    ```bash
    # assuming a temporary location is desired
    export ZITI_DATA=/tmp/ziti-data
    mkdir -p ${ZITI_DATA}/db
    ```

1. Crucially, the remaining examples assume that the name of the directory where this repo is checked-out is exactly "ziti".

### Initialize the Controller DB

Before you can run the controller you have to initialize it's database with an administrative user.

```bash
ziti-controller edge init ./etc/ctrl.with.edge.yml -u <admin name> -p <admin password>
```

### Run the Controller

Start the controller with the Ziti Fabric and Ziti Edge enabled (typical):

```bash
ziti-controller run etc/ctrl.with.edge.yml
```

Alternatively, you may start the controller Ziti Fabric standalone, without the Edge APIs:

```bash
ziti-controller run etc/ctrl.yml
```

Please note that if you start the controller without the Ziti Edge enabled then Ziti SDK and edge router functionality
will not be usable. The remainder of this article will assume you're running the controller with Edge enabled.

### Login to the Controller

This step will save a session token in the `ziti` CLI's configuration cache.

```bash
ziti edge login -u admin
```

Subsequent `ziti` CLI commands will automatically re-use this session token. If the token expires you'll need to perform this step again.

### Enroll the Routers

The Ziti Fabric requires at least one router (fabric router or edge router). There are four predefined configuration files
for running routers in `etc/` named `001.yml` to `004.yml`.

Each configuration file refers to certificate and private keys kept in `etc/ca/intermediate/certs` and
`etc/ca/intermediate/private/`. The steps for starting the router is to first register the router then
start it.

Register:

```bash
ziti fabric create router etc/ca/intermediate/certs/XXX-client.cert.pem
```

Where `XXX` is replaced with `001` through `004`.

### Start the Routers

```bash
ziti-router run etc/XXX.yml
```

Where `XXX` is replaced with `001` through `004`.

Edge routers are routers that have the Edge functionality enabled and allow Ziti SDK enabled application to connect to
Ziti. Starting an edge router requires that the controller be running with the Edge functionality
enabled. This requires the use of the `ctrl.with.edge.yml` configuration to run the controller (see example above).

To start an edge router the Edge REST API will be used to prime the enrollment process and the
`ziti-router enroll` command will be used to finalize the process. The enrollment command will handle adding the fabric
and edge router entries necessary. Using the `ziti fabric create router` command should not and can not be run.

There is only one example configuration file for an edge router:
`etc/edge.router.yml`. Additional configuration files can be created by copying and altering the
file. Specifically the identity section needs to point to unique file locations that do not collide with other identity
files, and the listening ports need to be available to bind.

## Ziti Edge REST API Primer

The Edge REST API is actually two APIs in one:

* `edge-client` REST API
* `edge-management` REST API

All API operations from both APIs are exposed on the default server listener unless the controller is configured to bind these two APIs separately. The API spec for each is served from the API itself. Read more about the spec [in the docs](https://openziti.github.io/api/rest/).

All of these example requests have analogous commands in the `ziti` CLI.

### Authenticate w/ the Ziti Edge API

```http
POST /authenticate?method=password
```

```json
{
    "username": "admin",
    "password": "admin"
}
```

Authentication will return a session token that should be supplied either as a cookie (also returned) or a HTTP header
called `zt-session`. All subsequent requests will either need to set the HTTP set `zt-session` or provide the cookie in
every request.

### Create An Edge Router

```http
POST /edge-routers
```

```json
{
  "name": "My Edge Router",
}
```

### Retrieve the Edge Router Enrollment Token

```http
GET /edge-routers/<id>
```

...where `id` is provided in the response to creating an edge router. It can be re-retrieved by listing the existing
edge routers via `GET /edge-routers`. The response from retrieving a edge router should contain an enrollment JWT in the 
`enrollmentJwt` field. Retain the enrollment JWT in a text file named `enrollment.jwt`.

### Enroll the Edge Router

```bash
ziti-router enroll --jwt <path to enrollment.jwt> etc/edge.router.yml
```

...where `path to enrollment.jwt` is the enrollment JWT for the edge router.

### Start Ziti Edge Router

```bash
ziti-router run etc/edge.router.yml
```

## Further Exploration

At this point the controller should be running with some number of routers running. It is now possible
to explore the Ziti Fabric capabilities via the `ziti fabric` command.

If the controller was started with the Edge functionality enabled the Ziti Edge API can be explored.

The Ziti Go SDK that is used by the apps in this repo can be found [in GitHub](https://github.com/openziti/sdk-golang).
