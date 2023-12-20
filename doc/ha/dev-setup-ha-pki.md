# Overview

This guide walks you through creating the certificates necessary to run a three node HA cluster.

**NOTE**: This folder contains a script `create-pki.sh` which will perform the steps outlined in
this guide for you.

## Create a CA

Create a self-signed certificate authority (CA) for the trust-root of your cluster

```
ziti pki create ca --trust-domain ha.test --pki-root ./pki --ca-file ca --ca-name 'HA Example Trust Root'
```

## Create Controller Certs

We are going to create an intermediate CA for each controller. We'll use this intermediate CA for
the following purposes:

1. To create a cert which will represent the controller. It will be used
    1. On the client side when dialing other controllers in the cluster
    2. On the server side when receiving connections from other controllers
    3. On the server side when receiving connections from routers
    4. On the server when handling REST API requests
2. To create identity certs as part of the identity enrollment process
3. To create router certs as part of the router enrollment process

### Notes

#### Client vs Server Certs

You may use separate certs and keys for client and server connections, but it's not necessary. When
you use a server cert on the client side it exposes information about what IPs and DNS entries the
cert is valid for, but since we're only connecting to other controllers, this should not be a
concern. However, the option to use separate certs is available, should you wish to use it.

#### REST Endpoint Certs

You may also use a different set of certs for the REST endpoint.

#### Sharing Signing Certs

You could use the same signing cert for all controllers in a cluster. However, if a signing cert is
ever compromised, all certs signed by the signing cert would need to be revoked. By using a separate
cert for each controller we limit the fallout from an individual controller or cert being
compromised.

### Create the Controller 1 signing and server certs

```shell
# Create the controller 1 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl1 --intermediate-name 'Controller One Signing Cert'

# Create the controller 1 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl1 --dns localhost --ip 127.0.0.1 --server-name ctrl1 --spiffe-id 'controller/ctrl1'

# Create the controller 2 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl2 --intermediate-name 'Controller Two Signing Cert'

# Create the controller 2 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl2 --dns localhost --ip 127.0.0.1 --server-name ctrl2 --spiffe-id 'controller/ctrl2'

# Create the controller 3 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl3 --intermediate-name 'Controller Three Signing Cert'

# Create the controller 3 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl3 --dns localhost --ip 127.0.0.1 --server-name ctrl3 --spiffe-id 'controller/ctrl3'
```
