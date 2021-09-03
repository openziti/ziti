# Introduction
This document contains more in-depth information for various Ziti topics. Most of this should be folded into the docs, but it's here as a resource for now.

## Edge Router Topologies
When developing or testing we may send all traffic through a single router. This is not how a Ziti network would commonly be deployed. Usually there would be multiple routers co-located with the application servers, hosted so that they were not reachable from the public internet. These routers would reach out to routers hosted on the public internet. Clients would connect to the public facing routers.

## Service Properties

Let’s take a look at each of the service properties:

### id

A system generated immutable ID. Different UIs like the CLI and the admin console will generally allow you to reference things by name. The ID just provides a stable unique ID, as opposed to the name, which must also be unique but can change.

### name

A user provided unique, mutable identifier. Generally used for display.

### encryption required

Ziti encrypts communications between processes when configured with TLS. This means data will be encrypted on the wire between the client and routers and in the context of router to router communication. Routers must decrypt data in memory in order to perform routing. This can be avoided, as Ziti SDKs also provide for transparent end-to-end encryption. This means that Ziti SDKs will encrypt your data before it enters a Ziti network, and it won't be decrypted until it is received on the other side. This flag controls whether this encryption is mandatory. If you set the flag on a service and end-to-end encryption handshaking fails, the SDKs will fail the connection rather than falling back to unencrypted communication.

### terminator strategy

This comes into play when you have multiple ways to route traffic to the application(s) hosting the service. This could be:

* Multiple application servers, either for a HA (high availability) or horizontal scaling
* Multiple routers configured to go to a single application server, for redundancy or performance of the Ziti network
* A combination of the two

The strategy determines how we pick which application gets a particular connection attempt via which route.
See [Ziti Services | Terminators](https://openziti.github.io/ziti/services/overview.html?tabs=create-service-ui#terminators) for more information.

### role attributes

Used by the policy system. We’ll cover this in more details shortly.

# Terminator
Taking a look at the results, the id, service, router and address properties should all make sense. Let’s take a brief look at some properties.

#### binding

Different terminators can be handled by different code in the routers. This tells the router how this terminator should be treated. An edge_transport binding directs the router to use a module which knows how to handle edge end-to-end encryption and connect to TCP and TLS addresses. We’ll see a different type of binding when we look at app-embedded hosting.

#### identity

Used by P2P or mesh type services, for example VoIP. Generally used when you have multiple applications all hosting the same service, and you need to be able to reach a specific one. For example, you may have many VoIP applications all hosting a VoIP service. When you dial the service, you'll want to connect to the specific application that the person you're trying to reach is running.

#### cost, precedence and dynamic-cost

These properties are all used by terminator strategies. We’ll cover them in the hosting tutorial.

Now that we have configured the service side of our service, let’s take a look at the client side.

# Tunnelers

If we have an existing client application which doesn’t have Ziti embedded, the Ziti project provides some applications which can bridge the gap between a plain client application and a Ziti-hosted service. They generally work by acting as a proxy, intercepting network traffic and redirecting it through Ziti. They can even intercept DNS names and IP addresses, so that client configuration doesn’t need to change. We're not going to demonstrate a tunneler as part of this tutorial, but know that there are options available for those cases where embedding an SDK into the application isn’t feasible, or you need an intermediary solution. 
