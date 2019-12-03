**This documentation requires revision after the migration to GitHub is complete.**


# Ziti Fabric Layer

The Ziti Fabric is a long-haul transport fabric for the Ziti ecosystem.

## Creating a Development Workspace

If you're going to be working on the Ziti fabric, or just want to track the latest development changes, you'll need a development workspace.

First, install the golang environment for your platform. Visit http://golang.org to download your installer.

As of this update, we're currently using version `1.13` of golang.

Make sure that `go` is in your path:

	$ go version
	go version go1.13.1 linux/amd64

You'll need to create a `GOPATH` for Ziti. I like to put mine in `~/Repos/nf/ziti`, but you can root it wherever you'd like. I'd suggest you call it `ziti`.

Essentially:

* `mkdir ziti`
* `cd ziti`
* `export GOPATH=$(pwd)`

When you've got your `GOPATH` ready, clone the repository.

* Clone this repository into `$GOPATH/src/bitbucket.org/netfoundry/ziti`
	* `mkdir -p $GOPATH/src/bitbucket.org/netfoundry`
	* `cd $GOPATH/src/bitbucket.org/netfoundry`
	* `git clone git@bitbucket.org:netfoundry/ziti`
	
Build the tree:

    $ go install ./...

The binaries will be placed in `$GOPATH/bin`.

The development configuration files live in `$GOPATH/src/github.com/netfoundry/ziti-fabric/fabric/etc`, and contain relative paths, which expect the executables to be started from `$GOPATH`.

    $ cd $GOPATH
    $ mkdir -p db
    $ bin/ziti-controller run src/github.com/netfoundry/ziti-fabric/fabric/etc/ctrl.yml

## Launching A Simple Environment

You'll want to open a number of terminal windows. All commands are executed relative to `$GOPATH` (shell PWD is `$GOPATH`).

### Launch The Controller

`$ bin/ziti-controller run src/github.com/netfoundry/ziti-fabric/fabric/etc/ctrl.yml`

### Configure Dotzeet

In order to use the `ziti-fabric` tool, you'll need a working identity configuration in your home directory. Create the file `~/.ziti/identities.yml` containing the following. Substitute your concrete `$GOPATH` for the actual value of your `$GOPATH` in this file:

```
default:
  caCert: "$GOPATH/src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/ca-chain.cert.pem"
  cert: "$GOPATH/src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/dotzeet-client.cert.pem"
  serverCert: "$GOPATH/src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/dotzeet-server.cert.pem"
  key: "$GOPATH/src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/private/dotzeet.key.pem"
  endpoint: tls:127.0.0.1:10000
```

Where, `"caCert": "$GOPATH/src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/ca-chain.cert.pem"` becomes `"caCert": "/home/michael/Repos/nf/ziti/src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/ca-chain.cert.pem"`.

The `endpoint:` specification should point at the `mgmt` listener address for your `ziti-controller`.

### Enroll Routers

With your controller running, use the `ziti-fabric` tool to enroll routers:

    $ bin/ziti-fabric create router src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/001-client.cert.pem
    $ bin/ziti-fabric create router src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/002-client.cert.pem
    $ bin/ziti-fabric create router src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/003-client.cert.pem
    $ bin/ziti-fabric create router src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/004-client.cert.pem

### Start Routers

With your controller running, you can now start routers to begin building your mesh:

`$ bin/ziti-router run src/github.com/netfoundry/ziti-fabric/fabric/etc/001.yml`

There are 4 router configurations provided (001, 002, 003, 004).

Start routers 001, 002, and 003.

The configuration provided in the tree assembles a "diamond" shaped mesh, where router `001` is intended to initiate 
(ingress) sessions, and router `003` is intended to terminate (egress) sessions. With smart routing and dynamic healing
in play, traffic can flow between router `001` and router `003` over either router `002` or `004`. 

### Create a Google Service

Create a service to access `google.com`:

`$ bin/ziti-fabric create service google tcp:google.com:80 003`

### Access the Google Service

Access the google service using `ziti-fabric`:

`$ bin/ziti-fabric-test http http://google --host www.google.com`

You should see HTTP output from the google website.

## Using The Fabric InfluxDB Metrics Reporter

First you'll want to launch an InfluxDB docker container like this:

    $ docker run --name influxdb -d -p 8086:8086 -v /opt/influxdb:/var/lib/influxdb influxdb
    
The directory `/opt/influxdb` should be changed to wherever you would like InfluxDB to persist its data.

Next, open an `influx` cli connection:

    $ docker exec -it influxdb influx
    Connected to http://localhost:8086 version 1.7.7
    InfluxDB shell version: 1.7.7
    >
    
Then create the `ziti` database:

    >  create database ziti
    >
    
Exit the `influx` cli. InfluxDB is ready to go.

Make sure you uncomment the `metrics` section in the `fabric/etc/ctrl.yml`:

    metrics:
      influxdb:
        enabled:            true
        url:                http://localhost:8086
        database:           ziti

Restart your controller and metrics should begin flowing into InfluxDB. You can verify like this:

    $ docker exec -it influxdb influx
    Connected to http://localhost:8086 version 1.7.7
    InfluxDB shell version: 1.7.7
    > use ziti
    Using database ziti
    > show series
    key
    ---
    egress.rx.bytesrate,source=001
    egress.rx.bytesrate,source=002
    egress.rx.bytesrate,source=003
    ----8<----
    link.OY8P.tx.msgsize,source=002
    link.OY8P.tx.msgsize,source=003
    link.yMey.latency,source=001
    link.yMey.latency,source=002
    link.yMey.rx.bytesrate,source=001
    link.yMey.rx.bytesrate,source=002
    link.yMey.rx.msgrate,source=001
    link.yMey.rx.msgrate,source=002
    link.yMey.rx.msgsize,source=001
    link.yMey.rx.msgsize,source=002
    link.yMey.tx.bytesrate,source=001
    link.yMey.tx.bytesrate,source=002
    link.yMey.tx.msgrate,source=001
    link.yMey.tx.msgrate,source=002
    link.yMey.tx.msgsize,source=001
    link.yMey.tx.msgsize,source=002
    > select * from "link.yMey.latency"
    name: link.yMey.latency
    time                count max    mean     min    p50      p75    p95    p99    p999   p9999  source stddev variance
    ----                ----- ---    ----     ---    ---      ---    ---    ---    ----   -----  ------ ------ --------
    1563471804584021939 1     615541 615541   615541 615541   615541 615541 615541 615541 615541 001    0      0
    1563471807396555092 1     549054 549054   549054 549054   549054 549054 549054 549054 549054 002    0      0
    1563471819583707653 2     630068 622804.5 615541 622804.5 630068 630068 630068 630068 630068 001    7263.5 52758432.25
    1563471822396356386 2     549054 506257   463460 506257   549054 549054 549054 549054 549054 002    42797  1831583209
    > select * from "link.yMey.latency"
    name: link.yMey.latency
    time                count max    mean     min    p50      p75    p95    p99    p999   p9999  source stddev            variance
    ----                ----- ---    ----     ---    ---      ---    ---    ---    ----   -----  ------ ------            --------
    1563471804584021939 1     615541 615541   615541 615541   615541 615541 615541 615541 615541 001    0                 0
    1563471807396555092 1     549054 549054   549054 549054   549054 549054 549054 549054 549054 002    0                 0
    1563471819583707653 2     630068 622804.5 615541 622804.5 630068 630068 630068 630068 630068 001    7263.5            52758432.25
    1563471822396356386 2     549054 506257   463460 506257   549054 549054 549054 549054 549054 002    42797             1831583209
    1563471834584197383 3     630068 555820   421851 615541   630068 630068 630068 630068 630068 001    94915.85098742289 9009018768.666666
    >

## Using The Fabric (JSON) Gateway

Launch the `ziti-fabric-gw` like this:

    $ bin/ziti-fabric-gw run src/github.com/netfoundry/ziti-fabric/fabric/etc/gw.yml

Client access like this:

    $ cd ${GOPATH}/src/bitbucket.org/netfoundry/ziti
    $ curl --cacert fabric/etc/ca/intermediate/certs/ca-chain.cert.pem --cert fabric/etc/ca/intermediate/certs/ctrl-client.cert.pem --key fabric/etc/ca/intermediate/private/ctrl.key.pem https://127.0.0.1:10080/ctrl/services | jq
      % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                     Dload  Upload   Total   Spent    Left  Speed
    100   443  100   443    0     0  88600      0 --:--:-- --:--:-- --:--:-- 88600
    {
      "data": [
        {
          "id": "google",
          "endpointAddress": "tls:www.google.com:443",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "googles",
          "endpointAddress": "tcp:www.google.com:443",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "loop",
          "endpointAddress": "tcp:127.0.0.1:8171",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "loop-broken",
          "endpointAddress": "tcp:127.0.0.1:8171",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "quigley",
          "endpointAddress": "tcp:one.quigley.com:80",
          "egress": "003",
          "binding": ""
        }
      ]
    }

To use `ziti-fabric-gw` with a plain HTTP listener (without TLS), comment out the `certPath` and `keyPath` lines in `gw.yml`:

    # Comment out the following lines to disable the TLS listener
    #certPath:         src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/certs/mgmt-gw.cert.pem
    #keyPath:          src/github.com/netfoundry/ziti-fabric/fabric/etc/ca/intermediate/private/mgmt-gw.key.pem

And then the listener can be accessed with plain HTTP:

    $ curl http://127.0.0.1:10080/ctrl/services | jq
      % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                     Dload  Upload   Total   Spent    Left  Speed
    100   443  100   443    0     0   432k      0 --:--:-- --:--:-- --:--:--  432k
    {
      "data": [
        {
          "id": "google",
          "binding": "",
          "endpointAddress": "tls:www.google.com:443",
          "egress": "003"
        }
      ]
    }


## Generating Network Load Using loop2

In order to create interesting metrics, you'll need to create some network load. A simple tool for that is the `ziti-fabric-test loop2` tool.

Create the `loop` service in the fabric (if it's not already there):

    $ bin/ziti-fabric create service loop tcp:127.0.0.1:8171 003

Launch a `loop2` listener (in $GOPATH):

    $ bin/ziti-fabric-test loop2 listener
    
Launch a `loop2` dialer (begin generating load):

    $ bin/ziti-fabric-test loop2 dialer src/github.com/netfoundry/ziti-fabric/fabric/etc/loop2/10-ambient.loop2.yml
    
Take a look at the various `loop2` scenario configurations in `fabric/etc/loop2` for examples of how to create a specific kind of workload.

## Creating Keys and Certificates for Development

This is not necessary if you are using the PKI infrastructure that is already configured in the development config files (above).

This is only necessary if you want to generate your own PKI infrastructure for your own configuration.
 
    ziti pki create --pki-root=/home/plorenz/tmp/002 ca --ca-name=root-ca --ca-file=root-ca
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=root-ca intermediate

    ziti pki create key --pki-root /home/plorenz/tmp/002/ --ca-name=intermediate --key-file ctrl
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=intermediate server --server-file=ctrl-server --ip 127.0.0.1 --key-file ctrl
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=intermediate client --client-file=ctrl-client --key-file ctrl

    ziti pki create key --pki-root /home/plorenz/tmp/002/ --ca-name=intermediate --key-file dotzeet
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=intermediate server --server-file=dotzeet-server --ip 127.0.0.1 --key-file dotzeet
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=intermediate client --client-file=dotzeet-client --key-file dotzeet

    ziti pki create key --pki-root /home/plorenz/tmp/002/ --ca-name=intermediate --key-file 002
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=intermediate server --server-file=002-server --ip 127.0.0.1 --key-file 002 --server-name 002
    ziti pki create --pki-root=/home/plorenz/tmp/002/ --ca-name=intermediate client --client-file=002-client --key-file 002 --client-name 002
