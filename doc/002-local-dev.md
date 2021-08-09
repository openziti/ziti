# Overview

This README aims to allow the setup of a development local environment as quick as possible. It uses predefined
configuration files that are maintained with the source code as well as Docker to run ancillary services (such as databases).

## Dependencies

- Go 1.12+

## Debugging

This guide can be used to run all of the Ziti applications via command line or in the debugger of an IDE.

## Get Started

### Checkout & Build

1. Checkout the `ziti` repository from `github.com/openziti/ziti`
    - `git clone https://github.com/openziti/ziti.git`
2. Change into the `ziti` dirrectory
    - `cd ziti`
3. Build
    - `go install ./...`

### Initializing the Controller
Before you can run the controller you have to initialize it's database with an administrative user. Assuming you want to run with the edge enabled, this can be done using as follows:

```bash
ziti-controller edge init etc/ctrl.with.edge.yml -u <admin name> -p <admin password>
```

Example:

```bash
ziti-controller edge init etc/ctrl.with.edge.yml -u admin -p o93wjh5n
```

### Starting the Controller

If you wish to start the controller with the Ziti Fabric and Ziti Edge enabled:

```bash
ziti-controller run etc/ctrl.with.edge.yml
```

If you wish to start the Ziti Fabric standalone:

```bash
ziti-controller run etc/ctrl.yml
```

Please note that if you start the controller without the Ziti Edge enabled, the Ziti SDK, and edge router functionality
will not be usable. The controller can be started and stopped both ways without issue.

### Starting  Routers

The Ziti Fabric requires at least one router (fabric router or edge router). There are four predefined configuration files 
for running routers in `etc/` named `001.yml` to `004.yml`. 

Each configuration file refers to certificate and private keys kept in `etc/ca/intermediate/certs` and
`etc/ca/intermediate/private/`. The steps for starting the router is to first register the router then 
start it.

Register:

```bash
ziti-fabric create router etc/ca/intermediate/certs/XXX-client.cert.pem
```

Where `XXX` is replaced with `001` through `004`.

Run:

```bash
ziti-router run etc/XXX.yml
```

Where `XXX` is replaced with `001` through `004`.

### Starting Router As An Edge Router

Edge routers are routers that have the Edge functionality enabled and allow Ziti SDK enabled application to connect to 
Ziti. Starting an edge eouter requires that the controller be running with the Edge functionality
enabled. This requires the use of the `ctrl.with.edge.yml` configuration to run the controller (see example above).

To start an edge router the Edge REST API will be used to prime the enrollment process and the
`ziti-router enroll` command will be used to finalize the process. The enrollment command will handle adding the fabric
and edge router entries necessary. Using the `ziti-fabric create router` command should not and can not be run.


There is only one example configuration file for an edge router: 
`etc/edge.router.yml`. Additional configuration files can be created by copying and altering the
file. Specifically the identity section needs to point to unique file locations that do not collide with other identity
file and the listening ports need to not be in use.

### Authenticate w/ the Ziti Edge API

Please note that this is not a complete API reference and all of these request have analogous commands in the Ziti CLI via the `ziti` binary.

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
to explore the Ziti Fabric capabilities via the `ziti-fabric` executable.

If the controller was started with the Edge functionality enabled the Ziti Edge API can be explored. A POSTMAN collection
can be found in `github.com/openziti/edge/controller/postman` and the Ziti SDK can be found in
`netfoundry/sdk-golang`. Additionally the `ziti-enroller` and `ziti-tunnel` command in this repository contain reference implementations.
