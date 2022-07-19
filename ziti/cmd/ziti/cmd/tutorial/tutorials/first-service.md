# Introduction

Hello. In this tutorial we’re going to go explore services, identities and polices.

Please note: this tutorial can be run interactively, by running 'ziti edge tutorial first-service'. It can also be viewed as a web page [here](https://github.com/openziti/ziti/blob/release-next/ziti/cmd/ziti/cmd/tutorial/tutorials/first-service.md). It may be convenient to be able to view both at the same time. The interactive version will save you a lot of typing or copy/pasting, but content may be easier to read in a web-browser.

<!---action:pause -->

## Goals
Let’s say you’re the founder of CloudEcho, the number one provider of echo services. When someone needs to hear back what they’ve said, you are here to make that happen. Any way that a customer needs an echo, be it TCP, UDP or HTTP based echo services, you’ve got them covered. You’ve decided to try integrating Ziti into your services and clients. We’re going to look at a few different ways to do that integration and explore different areas of the Ziti model to build an understanding of how to provide and consume services. In this tutorial we’re going look at how to Zitify the HTTP echo service.

<!---action:pause -->

## Prerequisites

First, let’s make sure you’ve got an environment running you can work with. We need a Ziti controller and at least one edge router to work with. If you don’t have a controller and edge router running,
you can use the quick-start script found [here](https://github.com/openziti/ziti/tree/release-next/quickstart). The fastest way to invoke the quick-start is to run

`source <(wget -qO- https://raw.githubusercontent.com/openziti/ziti/release-next/quickstart/docker/image/ziti-cli-functions.sh); expressInstall`

### Authenticate to the controller

Let's make sure we can connect with the controller. We're going to log in with the Ziti CLI, which will then let us run the rest of our operations.

NOTE: If you've used the quickstart, then your username and password can be found in `$HOME/.ziti/quickstart/$(hostname)/$(hostname).env`

```action:ziti-login allowRetry=true
ziti edge login
```

Ziti API sessions usually expire after a few minutes. In order to make sure that this session doesn't timeout while the tutorial is running, we're going to run a session keep-alive in the background.

```action:keep-session-alive interval=1m
If your session times out you can run ziti edge login again.
```

### Select Edge Router

Great, now that we’re logged in, let’s find our local edge router.

Note that this tutorial assumes that you’ve got an edge router running on the same machine as this tutorial. We'll be using localhost when telling the edge router how to connect to the tutorial services. If you don’t have an edge router running locally, please add one now.

```action:select-edge-router
Pick the name of an edge router to use. It will be referenced in this tutorial as ${edgeRouterName}
```

### Cleanup Tutorial Entities

Finally, in case you've run this tutorial before or have done other experimentation with this system the tutorial may not run cleanly. To ensure a clean run we're going to clean up any entities that would conflict with entities created as part of this tutorial.

```action:ziti
ziti edge delete service echo
ziti edge delete identities founder-laptop echo-server
ziti edge delete service-policies where true
ziti edge delete edge-router-policies where 'isSystem = false'
ziti edge delete service-edge-router-policies where true
```

# Setting up the Echo Service

We want to run our echo service over Ziti. We need to create various entities in Ziti for this to work, but the first is the service itself. The most important property of a Ziti service is its name. The service name is similar to a DNS name. When we connect to a service or host a service, we do so by name.

## Creating the Echo Service

Let’s create the echo service. The only required property we're providing for now is the name.

```action:ziti
ziti edge create service echo
```

Now that the service has been created, let’s query the service and see what shows up.

```action:ziti 
ziti edge list services 'name="echo"'
```

<!---action:pause -->

## Connecting Ziti to our Service

So, now Ziti has an echo service, but that service doesn’t go anywhere, and we don’t have any way to use it yet. We’re going to start by connecting the service to the applications hosting the service.

### Plain Echo Service

Let’s start by assuming we don’t want to make any changes to our server software yet. We’re just going to run an echo server which uses the standard library facilities. Let's start that service now. The source can be found [here](../plain_echo_server.go).

<!---action:show src=plain_echo_server.go highlight=go-->


```action:run-plain-echo-server
ziti edge tutorial plain-echo-server
```

The output for this service will be prefixed with plain-http-echo-server. The service is running on port ${port}. We can hit this service with a browser at

```
http://localhost:${port}?input=trees%20are%20tall 
```

to see the output. Or we can run it with the tutorial plain echo client. The client output will be prefixed with plain-echo-client.

The source for the plain client code is very simple, and can be found here: [here](../plain_echo_client.go)

<!---action:show src=plain_echo_client.go highlight=go-->

Let's run it and see what the output looks. At this point, we're not sending any traffic over the Ziti network.

```action:ziti templatize=true colorStdOut=false
ziti edge tutorial plain-echo-client --port ${port} trees are tall
```

<!---action:pause -->

### Terminators Overview

Now that we’ve got the service running, we can tell Ziti how to connect to it. We do that by creating a terminator for the service. A terminator represents an off ramp from the Ziti fabric. When we make a connection through Ziti to a service, the fabric will select a terminator to which traffic will be routed for that connection. For some types of terminators, each connection to the service will result in a new connection from the router to the hosting application. For others, an existing connection between the router and the hosting application will be used.

### Creating the Terminator

Let's create the terminator. The arguments to the create terminator command are:

1. **echo**: The service we’re creating the terminator for
1. **${edgeRouterName}**: The edge router we want to use to connect to the application server
1. **tcp:localhost:${port}**: The application server address. The format is protocol:hostname-or-ip:port. The protocol can be any of tcp, udp or tls. This address is what the edge router will try to make a
   connection to when someone uses the service.

```action:ziti templatize=true
ziti edge create terminator echo ${edgeRouterName} tcp:localhost:${port}
```

### Terminator Properties

Let's take a look at our new terminator:

```action:ziti failOk=true
ziti edge list terminators 'service.name="echo"'
```

<!---action:pause -->

# Client Application

In order to use the echo service, we need a client application. Our application could embed a Ziti SDK, but unmodified applications can also access Ziti services using a tunneler application. See [Tunnelers](https://openziti.github.io/ziti/clients/tunneler.html) for more information. Even though we're not covering tunnelers here, they are built with the Ziti SDKs. Therefore, all the following setup and configuration applies to them.

<!---action:pause -->

## SDK Client
Let's take a look at our SDK client code (viewable [here](../ziti_echo_client.go)). If you compare it with the plain version, you'll notice the actual client code is very similar. The main differences are

1. We have to load our configuration, so that we can initialize a Ziti Context
2. We have to intercept the Dial in the HTTP client, so we can send it through Ziti
3. Instead of host:port for the address, we use the service name 'echo'

<!---action:show src=ziti_echo_client.go highlight=go-->

We can't actually run it yet, because we don't have a configuration for it. As a next step, let's see how we can configure our client, so we can run it.

<!---action:pause -->

# Identity Setup

The main thing our client needs to connect to the Ziti network is an identity. One of the primary functions of Ziti is controlling access to services. In order for us to access the echo service, our client application needs an identity, so that we can permit that identity access.

## What is an Identity?

It's tempting to think of an identity as either a user or a device, but neither of those is entirely correct. A user may have multiple devices, each of which accesses the same service. In general a user will have a separate identity for each device from which they access the service. There are a few reasons for this approach:

* By enrolling each device individually we don’t transfer the credentials between devices. Because we generate the credentials each device, they can’t be intercepted.
* If someone compromises one device, we can disable just that identity, rather than everything for a given user.
* Traffic becomes attributable to a given device, which can be useful in a number of ways, including identifying misbehaving applications. 

If a user belongs to multiple Ziti networks, they will likely have multiple identities per device. In general, multiple identities on the same device for the same user will not cause any conflicts, unlike trying to run multiple VPNs concurrently. Even when using a Ziti tunnelers, each tunneler can host multiple identities concurrently and there will only be problems if multiple services try to claim the same DNS name or IP.
Creating the Identity

When creating an identity you can specify the identity type as being one of user, service or device. These types are currently just descriptive, and do not affect how Ziti uses the identities. Let’s say you’re running your own software on your laptop. Since you are the company founder, we’ll name the identity founder-laptop. We're also going to assign the identity a role attribute of management. We can use this later when setting up policies.

```action:ziti
ziti edge create identity user founder-laptop -o founder-laptop.jwt --role-attributes management
```

We have now created a new identity of type user with name founder-laptop. When Ziti created the identity, a one-time-token was also generated, which we’ve stored in founder-client.jwt. We’ll use this token to enroll our identity and create the configuration file for our client application.

Let’s take a look at our new identity:

```action:ziti
ziti edge list identities 'name="founder-laptop"'
```

In addition to the id, name and type we can see that identities, like services and edge routers, also have role-attributes. In the next section we'll be configuring service access, and we’ll see how role attributes come into play.

Before we can use our new identity, we need to enroll it. Enrolling the identity uses up the one-time-token and generates the configuration, keys and certificates that a client needs to use a Ziti network.

```action:ziti
ziti edge enroll -j founder-laptop.jwt -o founder-laptop.json
```

The resulting json file has the address of the Ziti controller, as well as the keys and certificates it needs to establish its identity and communicate securely with that controller.

As you saw, the usual flow for enrolling an application in a ziti network has two steps. However, unlike in our approach here, we don’t generally rely on the Ziti CLI to enroll applications and generate a config file. A more conventional flow would be:

1. Create an identity for the application, which includes generating a JWT
1. The application imports JWT, enrolls in the application and stores the configuration in its own format.
    1. Two ways that applications can import JWTs are by reading a file or reading a QR code

The reason enrollment is broken into two steps is because we only want the keys and certificates to exist on their final destination. We may transmit the JWT, but once enrollment has completed the JWT cannot be re-used. If someone later steals the JWT, they won’t be able to enroll, as the JWT will no longer be valid. If some intercepts the JWT and uses it to enroll before the intended recipient does, the intended recipient’s enrollment will fail. This lets us know that something is wrong, we can disable the identity, and the intrusion will have been quickly detected.

<!---action:pause-->

## Trying the Ziti Client
Now that we’re enrolled, let’s try to use our echo service.

```action:ziti failOk=true
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

Hopefully it comes as no surprise that the echo service wasn’t found, given that we haven’t granted our identity any access yet.

# Policy Configuration

The Ziti CLI comes with a tool called policy-advisor to help you figure out if you have correctly configured your policies. Let’s try it out. It can be run either from a service or identity-centric perspective. This following command will check if the founder-laptop identity can access the echo service.

```action:ziti
ziti edge policy-advisor identities -q founder-laptop echo
```

It should give us three errors.

1. That the identity has no access to the service
1. That the identity has no access to edge routers
1. That the service has no access to edge routers

Let’s tackle these one at a time.

**Note:** If you don’t see all of those errors, you most likely have other policies affecting your identity or service.

## Service Policy Setup

First we need to grant access to use the service via a service policy. There are two types of service policies:

* Dial - allow using a service
* Bind - allow hosting a service

For now, we just need a dial policy. Later, when we try hosting the echo service with an SDK embedded application, we’ll need a bind policy as well. We're going to explicitly add our service and identity to this policy. The service we'll reference by name. The identity we'll include by role attribute. 

For a deep dive into policies, see [here](https://openziti.github.io/ziti/policies/overview.html).

```action:ziti
ziti edge create service-policy echo-clients Dial --service-roles '@echo' --identity-roles '#management'
```

Let’s check and make sure it’s there:

```action:ziti
 ziti edge list service-policies 'name="echo-clients"'
```

If we run the policy-advisor, we should see that we have fixed the first error, and that we have Dial permissions for the echo service.

```action:ziti
ziti edge policy-advisor identities -q founder-laptop echo
```

If we try to run the client again, it still won’t work, but we should see a different error:

```action:ziti failOk=true
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

This error indicates that we have access to the service, but not to any edge routers.

<!---action:pause-->

# Edge Router Policy Setup

Now we need to give our client access to one or more edge routers. Edge routers are how client traffic enters a Ziti network. We need to be able to limit which identities can access specific edge routers. Certain edge routers may be dedicated to specific regions, users or services. 

```action:ziti templatize=true
ziti edge create edge-router-policy echo-clients --edge-router-roles '@${edgeRouterName}' --identity-roles '#management'
```

Taking a look, we should see now see our new edge router policy in place. 

```action:ziti
ziti edge list edge-router-policies 'name="echo-clients"'
```

If we run the policy advisor again, we should see a change.

```action:ziti
ziti edge policy-advisor identities -q founder-laptop echo
```

We should only have one error left, letting us know that the service doesn’t have any edge routers available to it.

If we run the client again, we’ll see the same error:

```action:ziti failOk=true
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

This is because the edge routers that can be used when establishing a session is the set of edge routers that the identity and service have in common. 

Looking at an example:

1. We have an identity which has access to edge routers A, B, C 
2. We have a service which can be accessed via edge routers C, D, E
3. That identity can only access the service via edge router C, since that's the only one they have in common

Even though our identity now has access to our edge router, our service does not. 

<!---action:pause-->

# Service Edge Router Policies
In the same way that we can assign edge routers to users, we can also assign them to services. You may wish to do this to conform with local regulations or to give services exclusive access to certain edge routers to meet SLAs. 

Since we don’t have any special requirements right now, let’s let the echo service use all edge routers. There’s a special role attribute of #all which will match all entities of a given type.

```action:ziti
ziti edge create service-edge-router-policy echo --edge-router-roles '#all' --service-roles '@echo'
```

Now if we run the policy advisor again, we should not see any errors.

```action:ziti
ziti edge policy-advisor identities -q founder-laptop echo
```

We should now finally be able to run the client and get some validation.

```action:ziti colorStdOut=false
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

We have our first successful connection through Ziti! **\o/**

<!---action:pause-->

# SDK-Embedded Server

Now that we’ve gotten our end-to-end test working let’s replace our plain server with a Zitified version.

## Clean up plain server

Before configuring and running our ziti-embedded server, we're going to stop the plain server and remove any related configuration.

```action:stop-plain-echo-server
Stop the plain echo server
```

We’ve stopped the echo server. If we now run the client, we’ll see an error saying that dialing the service fails.

```action:ziti colorStdOut=false failOk=true
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

We’re going to also remove the terminator, since that now points to an address where nothing is running.

```action:ziti
ziti edge delete terminators where 'service.name="echo"'
``` 

If we now run the client, we’ll see an error saying that the service has no terminators.

```action:ziti colorStdOut=false failOk=true
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

## Hosting Identity

Since we’re going to use the SDK in the server, we need an identity for the server.

```action:ziti
ziti edge create identity service echo-server -o echo-server.jwt --role-attributes echo-server
```

We’ll then enroll the server identity.

```action:ziti
ziti edge enroll -j echo-server.jwt -o echo-server.json
```

## Ziti Server Code

If you look at the server code, viewable [here](../ziti_echo_server.go), you should notice very few changes from the plain echo server. The actual request processing code is unchanged. 

<!---action:show src=ziti_echo_server.go highlight=go-->

Let’s try running it and see what happens.

```action:ziti colorStdOut=false failOk=true
ziti edge tutorial ziti-echo-server --identity echo-server.json
```

Since we haven’t given this identity access to the server, it fails.

## Server Side Policy Configuration

Let’s run the policy advisor first and see what it has to say.

```action:ziti
ziti edge policy-advisor identities -q echo-server echo
```

We should only see two errors.

* The identity does not have access to  the service
* The identity does not have access to any edge routers

We already granted the echo service to all edge routers, so that is no longer an issue.

First, let’s grant the identity access to the service. Unlike before, we’re not granting access to use the service, but to host it. So instead of a Dial policy, we need to create a Bind policy.

```action:ziti
ziti edge create service-policy echo-servers Bind --service-roles '@echo' --identity-roles '#echo-server'
```

We now have two service policies, one for clients and one for hosts

```action:ziti
ziti edge list service-policies 'name contains "echo"'
```

Running the policy advisor again, we should be down to one error:

```action:ziti
ziti edge policy-advisor identities -q echo-server echo
```

Finally, we create an edge router policy for the hosting identity. In this case, it has the same servers as the client side, but generally client side and hosting side identities will use different sets of edge routers.

```action:ziti templatize=true
ziti edge create edge-router-policy echo-servers --edge-router-roles '@${edgeRouterName}' --identity-roles '#echo-server'
```

We should now get a clean bill of health from the policy advisor

```action:ziti
ziti edge policy-advisor identities -q echo-server echo
```

Let’s give it a try now and see what happens:

```action:run-ziti-echo-server
ziti edge tutorial ziti-echo-server
```

Our client should work correctly. Hang on, you may be saying… won’t it fail with a ‘no terminators’ error? After all, we haven’t created a new terminator yet. Well, let’s take a look at our terminators:

```action:ziti failOk=true
ziti edge list terminators 'service.name="echo"'
```

There’s a terminator present, which looks quite different from the one we created manually. This terminator was created dynamically when the ziti-echo-server application bound the service. Client connections for echo will now get made over the existing network connection that the server application has with the edge router. When you stop the server application, the terminator will be removed.

This has some interesting benefits. For example, this can make horizontal scale easier to accomplish. When you spin up new instances of the server application, they can connect, dynamically create a terminator and start receiving a portion of the service traffic. When you stop an application, or it dies, the connection goes away, and the terminator is automatically removed. New connections won’t try to connect that application server anymore.

Hosting configuration and topologies is a big topic. For now, let’s try out our client with our single server instance running:

```action:ziti colorStdOut=false
ziti edge tutorial ziti-echo-client --identity founder-laptop.json trees are tall
```

Success, we now have a Ziti SDK-embedded client, communicating with a Ziti SDK-embedded server.  

Thanks for taking the time to learn about Ziti services, identities and policies!

Hopefully you’ve gotten a better idea of:

* How to create and manage services and identities 
* How terminators connect the Ziti fabric with application servers
* How to manage client access with identities and service policies
* How edge routers can be assigned across identities with edge router policies
* How edge routers can be assigned across services with service edge router polices

   
See the [Documentation Hub](https://openziti.github.io/) for more resources.
