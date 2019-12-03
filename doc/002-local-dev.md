# Overview

This README aims to allow the setup of a development local environment as quick as possible. It uses predefined
configuration files that are maintained with the source code as well as Docker to run ancillary services (such as 
databases).

# Dependencies

- Go 1.12+
- PostgresSQL v10+ (if running Ziti Edge)


# Debugging

This guide can be used to run all of the Ziti applications via command line or in the debugger of an IDE. The only
requirement is that the CLI start in/the debuggers current working directory is assumed to be your $GOPATH.


# Get Started

### Checkout & Build

Checkout the entire `ziti` repot to a `$GOPATH/src/bitbucket.org/netfoundry` directory.

Build the entire project and all binaries or just the Ziti Edge portion:

To install all: ```go install bitbucket.org/netfoundry/ziti/...```

...which will build and install the binaries into `$GOPATH/bin`. All future `ziti` commands will run binaries from here.

### Start PostgreSQL

If Ziti Edge functionality will be used it will be necessary to start a PostgreSQL database. This database can be on
any machine, but the default configuration file assume that the database is running on localhost.

```
docker run -it --name local-postgres -e POSTGRES_PASSWORD=ztpassword -p 5432:5432 postgres
```

Please note that the default `ztpassword` exists in the `edge.router.yml` file used below to start a router 
running with Edge functionality enabled.

### Starting the Controller

If you wish to start the controller with the Ziti Fabric and Ziti Edge enabled:

```
ziti-controller run $GOPATH/src/github.com/netfoundry/ziti-fabric/etc/ctrl.with.edge.yml
```


If you wish to start the Ziti Fabric standalone:

```
ziti-controller run $GOPATH/src/github.com/netfoundry/ziti-fabric/etc/ctrl.yml
```

Please note that if you start the controller without the Ziti Edge enabled, the Ziti SDK, and edge router functionality
will not be usable. The controller can be started and stopped both ways without issue. The only requirement is that the
PostgrSQL database must be running if the Ziti Edge functionality is enabled.


# Starting  Routers

The Ziti Fabric requires at least one router (fabric router or edge router). There are four predefined configuration files 
for running routers in `ziti/fabric/etc/` named `001.yml` to `004.yml`. 

Each configuration file refers to certificate and private keys kept in `ziti/fabric/etc/ca/intermediate/certs` and
`ziti/fabric/etc/ca/intermediate/private/`. The steps for starting the router is to first register the router then 
start it.

Register:

```
ziti-fabric create router $GOPATH\src\bitbucket.org\netfoundry\ziti\fabric\etc\ca\intermediate\certs\XXX-client.cert.pem
```

Where `XXX` is replaced with `001` through `004`.

Run:

```
ziti-router run $GOPATH/src/github.com/netfoundry/ziti-fabric/etc/XXX.yml
```

Where `XXX` is replaced with `001` through `004`.

# Starting Router As An Edge Router

Edge routers are routers that have the Edge functionality enabled and allow Ziti SDK enabled application to connect to 
Ziti. Starting an edge eouter requires that the controller be running with the Edge functionality
enabled. This requires the use of the `ctrl.with.edge.yml` configuration to run the controller (see example above).

To start an edge router the Edge REST API will be used to prime the enrollment process and the
`ziti-router enroll` command will be used to finalize the process. The enrollment command will handle adding the fabric
and edge router entries necessary. Using the `ziti-fabric create router` command should not and can not be run.


There is only one example configuration file for an edge router: 
`ziti/fabric/etc/edge.router.yml`. Additional configuration files can be created by copying and altering the
file. Specifically the identity section needs to point to unique file locations that do not collide with other identity
file and the listening ports need to not be in use.

### Authenticate w/ the Ziti Edge API

```
POST /authenticate?method=password
{
    "username": "admin",
    "password": "admin"
}
```

Authentication will return a session token that should be supplied either as a cookie (also returned) or a HTTP header
called `zt-session`. All subsequent requests will either need to set the HTTP set `zt-session` or provide the cookie in
every request.


### Create a Cluster / List Clusters

A `cluster id` is necessary to create an edge router. A cluster must be created or if one already exists, it can be used.

List:

```
GET /clusters
```

Create:

```
POST /clusters
{
  "name": "My Cluster",
}
```

### Create A Cluster

```
POST /clusters
{
  "name": "My Cluster",
}
```

A `clustet id` will be provided in the response as `id`. Retain for later use.

### Create An Edge Router
```
POST /edge-routers
{
  "name": "My Edge Router",
  "clusterId": "<cluster id>"
}
```

...where `cluster id` is replaced with a valid cluster id that already exists or was created. It can be re-retrieved by
listing the existing clusters via `GET /clusers`. The response from creating an edge router should contain an id in 
the `id` field. Retain for later use.

### Retrieve the Edge Router Enrollment Token

```
GET /edge-routers/<id>
```

...where `id` is provided in the response to creating an edge router. It can be re-retrieved by listing the existing
edge routers via `GET /edge-routers`. The response from retrieving a edge router should contain an enrollment JWT in the 
`enrollmentJwt` field. Retain the enrollment JWT in a text file named `enrollment.jwt`.

### Enroll the Edge Router

```
ziti-router enroll --jwt <path to enrollment.jwt> $GOPATH/src/github.com/netfoundry/ziti-fabric/etc/edge.router.yml
```

...where `path to enrollment.jwt` is the enrollment JWT for the edge router.

### Start Ziti Edge Router

```
ziti-router run $GOPATH/src/github.com/netfoundry/ziti-fabric/etc/edge.router.yml
```

# Further Exploration

At this point the controller should be running with some number of routers running. It is now possible
to explore the Ziti Fabric capabilities via the `ziti-fabric` executable. 

If the controller was started with the Edge
functionality enabled the Ziti Edge API can be explored. A POSTMAN collection can be found in
 `ziti/edge/controller/postman` and the Ziti SDK can be found in `ziti/sdk`. Additionally the `ziti-enroller`, `ziti-tunnel` and 
 `ziti-proxy` have the Ziti SDK embedded within them to act as a starting point for using Ziti end-to-end.