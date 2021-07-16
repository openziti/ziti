
# Ziti Overview

Ziti is split into two main domains that run in the same process space:

- Ziti Fabric
- Ziti Edge

## Fabric & Edge

The Ziti Fabric is a core set of features used to support defining and managing services, routers, and
sessions to route traffic across a robust and secure overlay network. Ziti Fabric features are always enabled and
cannot be disabled.

The Ziti Edge is a set of features that can be enabled on top of the Ziti Fabric features to enable enrollment
and management of endpoints that make use of the Ziti SDK. The Ziti SDK can be built into applications to provide
ingress and egress to the Ziti overlay network as well as to provide application specific networking to an individual
application. Enabling the Edge features is optional.

Both the Fabric and Edge features are built into the ziti-controller and ziti-router binaries.

## Ziti Controller

The Ziti Controller (ziti-controller) is the main server component of a Ziti environment. It is the first piece of Ziti
that must be setup and configured. The controller houses all the router, service, and management data necessary
to run a Ziti environment. There is one, and only one, controller per Ziti environment.

The Ziti Controller can optionally host the Ziti Edge features. The Fabric features within the controller
supports managing routers, services, and creating circuits across a mesh network to route traffic, but does not support
accepting connections from endpoints utilizing the Ziti SDK, provide a configurable policy management for endpoint
connectivity, and endpoint enrollment.

## Ziti Router

 The Ziti Router binary (ziti-router) is deployed multiple times to stand up multiple ingress and egress
 points for a Ziti overlay network. Each router has its own identity and must be enrolled with the controller.
 A Ziti environment requires one or more routers.

 If the Ziti Edge features are enabled, routers may optionally be enrolled as an "edge router". Edge routers allow Ziti
 SDK enabled applications, Ziti Applications, to access services or host services that have been configured within Ziti
 as overlay services.

## Ziti Applications

Below is an outline of all the applications that are generated from this repository.

### Servers

The following binaries are used to deploy long running servers that route traffic and manage the
configuration of a Ziti environment.

| Binary Name       | Description|
|-------------------| -----------|
| ziti-controller   | Runs a central server necessary for Ziti environments|
| ziti-router       | Runs a server capable of ingress'ing and egress'ing Ziti traffic standalone or as a mesh|

### Tools

The following binaries provide utility or testing functionality.

| Binary Name       | Description|
|-------------------| -----------|
| ziti-enroller     | Provides enrollment processing features for executables that do not directly support enrollment
| ziti-fabric-gw    | Provides JSON RCP web service access to Ziti fabric management features
| ziti-fabric-test  | The Ziti Fabric Toolbox which is used to test deployed fabric components|

### Management

The following binaries are used to configure and manage a Ziti environment via command line interactions.

| Binary Name       | Description|
|-------------------| -----------|
| ziti-fabric       | Provides command line access to Ziti Fabric management features|
| ziti              | Provides command line access to Ziti management features|

## Endpoint Clients

The following binaries are Ziti endpoint clients which have the Ziti SDK built into them and can connected to an
edge router. Endpoint clients can be application specific or act as a bridge to other applications, hosts, or underlay
networks.

| Binary Name       | Description|
|-------------------| -----------|
| ziti-tunnel       | Provides the ability to intercept traffic to route traffic across Ziti|

All of the above binaries are cross platform compatible, except ziti-tunnel which is currently Linux only.
