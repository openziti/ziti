![OpenZiti Logo](https://raw.githubusercontent.com/openziti/ziti-doc/main/docusaurus/static/img/ziti-logo-dark.svg)

<br>

[![Build Status](https://github.com/openziti/ziti/actions/workflows/main.yml/badge.svg?query=branch%3Arelease-next)](https://github.com/openziti/ziti/actions/workflows/main.yml?query=branch%3Arelease-next)
[![Go Report Card](https://goreportcard.com/badge/github.com/openziti/ziti)](https://goreportcard.com/report/github.com/openziti/ziti)
[![GoDoc](https://godoc.org/github.com/openziti/ziti?status.svg)](https://pkg.go.dev/github.com/openziti/ziti)
[![Discourse Widget](https://img.shields.io/badge/join-us%20on%20discourse-gray.svg?longCache=true&logo=discourse&colorB=brightgreen")](https://openziti.discourse.group/)
[![License: Apache-v2](https://img.shields.io/badge/License-Apache--2.0-yellow.svg)](LICENSE)

<br>

# OpenZiti

OpenZiti represents the next generation of secure, open-source networking for your applications. OpenZiti has several components.

## Quick Reference

* [Documentation](https://openziti.io/docs/learn/introduction/)
* [Developer Overview](./doc/001-overview.md)
* [Local Development Tutorial](./doc/002-local-dev.md)
* [Local Deployment Tutorial](./doc/003-local-deploy.md)
* [Controller PKI Tutorial](./doc/004-controller-pki.md)
* [Release Notes](./CHANGELOG.md)

---

## What is OpenZiti?

* The OpenZiti fabric provides a scalable, pluggable, networking mesh with built in smart routing
* The OpenZiti edge components provide a secure, Zero Trust entry point into your network
* The OpenZiti SDKs allow you to integrate OpenZiti directly into your applications
* The OpenZiti tunnelers and proxies allow existing applications and networks to take advantage of a OpenZiti deployment

### Security Features

* Zero Trust and Application Segmentation
* Dark Services and Routers
* End to end encryption

### Performance and Reliability

* A scalable mesh fabric with smart routing
* Support for load balancing services for both horizontal scale and failover setups

### Developer Focus

* [Open source code, available with the Apache 2.0 license](https://github.com/openziti)
* Fully programmable REST management APIs
* [SDKs for a variety of programming languages](https://openziti.io/docs/reference/developer/sdk)
* [Application specific configuration store allowing centralized management of configuration allowing you to add structured configuration specific to your application](https://openziti.io/docs/learn/core-concepts/config-store/overview)
* An extensible fabric, allowing you to add your own
  * load balancing algorithms
  * interconnect protocols
  * ingress and egress protocols
  * metrics collections frameworks
  * control and management plane messaging and semantics

### Easy Management

* [A flexible and expressive policy model for managing access to services and edge routers](https://openziti.io/docs/learn/core-concepts/security/authorization/policies/overview)
* A web based admin console
* [Pre-built tunnelers and proxies for a variety of operating systems, including mobile](https://openziti.io/docs/reference/tunnelers)

Let's break some of these buzzwords down.

### Zero Trust/Application Segmentation

Many networking security solutions act like a wall around an internal network. Once you are through the wall, you have access to everything inside. Zero trust solutions enforce not just access to a network, but access to individual applications within that network.

Every client in a OpenZiti system must have an identity with provisioned certificates. The certificates are used to establish secure communications channels as well as for authentication and authorization of the associated identity. Whenever the client attempts to access a network application, OpenZiti will first ensure that the identity has access to the application. If access is revoked, open network connections will be closed.

This model enables OpenZiti systems to provide access to multiple applications while ensuring that clients only get access to those applications to which they have been granted access.

In addition to requiring cert based authentication for clients, OpenZiti uses certificates to authorize communication between OpenZiti components.

### Dark Services and Routers

There are various levels of accessibility a network application/service can have.

1. Many network services are available to the world. The service then relies on authentication and authorization policies to prevent unwanted access.
1. Firewalls can be used to limit access to specific IP or ranges. This increases security at the cost of flexibility. Adding users can be complicated and users may not be able to easily switch devices or access the service remotely.
1. Services can be put behind a VPN or made only accessible to an internal network, but there are some downsides to this approach.
    1. If you can access the VPN or internal network for any reason, all services in that VPN become more vulnerable to you.
    1. VPNs are not usually appropriate for external customers or users.
    1. For end users, VPNs add an extra step that needs to be done each time they want to access the service.
1. Services can be made dark, meaning they do not have any ports open for anyone to even try and connect to.

Making something dark can be done in a few ways, but the way it's generally handled in OpenZiti is that services reach out and establish one or more connections to the OpenZiti network fabric. Clients coming into the fabric can then reach the service through these connections after being authenticated and authorized.

OpenZiti routers, which make up the fabric, can also be dark. Routers located in private networks will usually be made dark. These routers will reach out of the private network to talk to the controller and to make connections to join the network fabric mesh. This allows the services and routers in your private networks to make only outbound connections, so no holes have to be opened for inbound traffic.

Services can be completely dark if they are implemented with a OpenZiti SDK. If this is not possible a OpenZiti tunneler or proxy can be colocated with the service. The service then only needs to allow connections from the local machine or network, depending on how close you colocate the proxy to the service.

### End to End Encryption

If you take advantage of OpenZiti's developer SDKs and embed OpenZiti in your client and server applications, your traffic can be configured to be seamlessly encrypted from the client application to server application. If you prefer to use tunnelers or proxy applications, the traffic can be encrypted for you from machine to machine or private network to private network. Various combinations of the above are also supported.

End-to-end encryption means that even if systems between the client and server are compromised, your traffic cannot be decrypted or tampered with.

---

## Getting started with OpenZiti

If you are looking to jump right in feet first you can follow along with one of our [up-and-running quickstart
guides](https://openziti.io/docs/learn/quickstarts/). These guides are designed to get an
overlay network quickly and allow you to run it all locally, use Docker or host it anywhere.

This environment is perfect for evaluators to get to know OpenZiti and the capabilities it offers.  The environment was not
designed for large scale deployment or for long-term usage. If you are looking for a managed service to help you run a
truly global, scalable network browse over [the NetFoundry web site](https://netfoundry.io) to learn more.

## Build from Source

Please refer to [the local development tutorial](./doc/002-local-dev.md) for build instructions.

---

## Adopters

Interested to see what companies are using OpenZiti? Check out [the list of projects and companies using OpenZiti here](./ADOPTERS.md).
Interested in adding your project to the list? Add an issue to github or better yet feel free to add a pull request! Instructions for
getting your project added are included [on the adopters list](./ADOPTERS.md)

---

## Support

We have a very active Discourse forum. Join the conversation! Help others if you can. If you want to ask a question or just check it out,
cruise on over to [the OpenZiti Discourse forum](https://openziti.discourse.group/). We love getting questions, jump in!

---

### Contributing

The OpenZiti project welcomes contributions including, but not limited to, code, documentation and bug reports.

* All OpenZiti code is found on Github under the [OpenZiti](https://github.com/openziti) organization. 
  * [ziti](https://github.com/openziti/ziti): top level project which builds all OpenZiti executables
  * [edge](https://github.com/openziti/edge): edge components and model which includes identity, polices and config 
  * [fabric](https://github.com/openziti/fabric): fabric project which includes core controller and router
  * [foundation](https://github.com/openziti/foundation): project which contains library code used across multiple projects
  * SDKs
    * [ziti-sdk-c](https://github.com/openziti/ziti-sdk-c): C SDK
    * [sdk-golang](https://github.com/openziti/sdk-golang): Go SDK
    * [ziti-sdk-jvm](https://github.com/openziti/ziti-sdk-jvm): SDK for JVM based languages
    * [ziti-sdk-swift](https://github.com/openziti/ziti-sdk-swift): Swift SDK
    * [ziti-sdk-nodejs](https://github.com/openziti/ziti-sdk-nodejs): NodeJS SDK
    * [ziti-sdk-csharp](https://github.com/openziti/ziti-sdk-csharp): C# SDK
  * [ziti-doc](https://github.com/openziti/ziti-doc): Powers the static documentation site

OpenZiti was developed and open sourced by [Netfoundry, Inc](https://netfoundry.io). NetFoundry continues to fund and 
contribute to OpenZiti.
