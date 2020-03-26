## Creating a Development Workspace for ziti-fabric Development

If you're going to be working on the Ziti fabric, or just want to track the latest development changes, you'll need a development workspace.

First, install the golang environment for your platform. Visit http://golang.org to download your installer.

As of this update, we're currently using version `1.13` of golang.

Make sure that `go` is in your path:

```
$ go version
go version go1.13.4 linux/amd64
```

Consider using a separate directory to contain the `GOPATH` for your `ziti-fabric` development (instead of just using `~/go`). I like to put mine in `~/local/ziti-fabric`, but you can root it wherever you'd like. This will contain the dependent package sources, and built binaries.

Essentially:

```
mkdir -p ~/local/ziti-fabric
export GOPATH=~/local/ziti-fabric
```

Include `GOPATH/bin` in your shell's `PATH`:

```
$ export PATH=$GOPATH/bin:$PATH
```

When you've got your `GOPATH` ready, you'll want to clone the repositories. I like to keep my repositories in `~/repos`.

```
mkdir ~/repos
cd ~/repos
git clone https://github.com/netfoundry/ziti-cmd.git
git clone https://github.com/netfoundry/ziti-fabric.git
```

We're going to need to update the `go.mod` file in the root of `ziti-cmd`, using the `replace` directive to point the `ziti-cmd` build at our local `ziti-fabric` development tree.

```
cd ~/repos/ziti-cmd
vi go.mod
```

The top of the file should look like this:

```
module github.com/netfoundry/ziti-cmd

go 1.13

require (
```

We're going to add a `replace` line, like this:

```
module github.com/netfoundry/ziti-cmd

go 1.13

replace github.com/netfoundry/ziti-fabric => ../ziti-fabric

require (
```
	
With that change made, you can alter the contents of your local clone of `ziti-fabric`, and builds of `ziti-cmd` will use your local changes, rather than the version it pulled into `GOPATH` from GitHub.

Build the tree:

```
$ cd ~/repos/ziti-cmd
$ go install ./...
```

The binaries will be placed in `$GOPATH/bin`.

The development configuration files live in `ziti-fabric/etc`, and contain relative paths, which expect the executables to be started from the root of `ziti-fabric` (ensure that `~/local/ziti-fabric/bin` (`$GOPATH/bin`) is in your shell's `PATH`).

```
$ cd ~repos/ziti-cmd
$ ziti-controller run etc/ctrl.yml
```

## Launching A Simple Environment

You'll want to open a number of terminal windows. All commands are executed relative to `~/repos/ziti-fabric`.

### Launch The Controller

```
$ cd ~repos/ziti-cmd
$ ziti-controller run etc/ctrl.yml
```

### Configure Dotzeet

In order to use the `ziti-fabric` tool, you'll need a working identity configuration in your home directory. Create the file `~/.ziti/identities.yml` containing the following. Substitute your concrete `$GOPATH` for the actual value of your `$GOPATH` in this file:

```
default:
  caCert: "$HOME/repos/ziti-cmd/etc/ca/intermediate/certs/ca-chain.cert.pem"
  cert: "$HOME/repos/ziti-cmd/etc/ca/intermediate/certs/dotzeet-client.cert.pem"
  serverCert: "$HOME/repos/ziti-cmd/etc/ca/intermediate/certs/dotzeet-server.cert.pem"
  key: "$HOME/repos/ziti-cmd/etc/ca/intermediate/private/dotzeet.key.pem"
  endpoint: tls:127.0.0.1:10000
```

You'll want to replace `$HOME` in the above with the contents of your `HOME` environment variable. Im my case, this becomes `/home/michae/repos/ziti-fabric/`...

The `endpoint:` specification should point at the `mgmt` listener address for your `ziti-controller`.

### Enroll Routers

With your controller running, use the `ziti-fabric` tool to enroll routers:

```
$ ziti-fabric create router etc/ca/intermediate/certs/001-client.cert.pem
$ bin/ziti-fabric create router etc/ca/intermediate/certs/002-client.cert.pem
$ bin/ziti-fabric create router etc/ca/intermediate/certs/003-client.cert.pem
$ bin/ziti-fabric create router etc/ca/intermediate/certs/004-client.cert.pem
```

### Start Routers

With your controller running, you can now start routers to begin building your mesh:

```
$ ziti-router run etc/001.yml
```

There are 4 router configurations provided (`001`, `002`, `003`, `004`).

Start routers `001`, `002`, `003`, and `004`.

The configuration provided in the tree assembles a "diamond" shaped mesh, where router `001` is intended to initiate (ingress) sessions, and router `003` is intended to terminate (egress) sessions. With smart routing and dynamic healing in play, traffic can flow between router `001` and router `003` over either router `002` or `004`. 

### Create a Google Service

Create a service to access `google.com`:

```
$ ziti-fabric create service google
$ ziti-fabric create terminator google 003 tcp:google.com:80
```

### Access the Google Service

Access the google service using `ziti-fabric`:

```
$ ziti-fabric-test http http://google --host www.google.com
```

You should see HTTP output from the google website.

## Generating Network Load Using loop2

In order to create interesting metrics, you'll need to create some network load. A simple tool for that is the `ziti-fabric-test loop2` tool.

Create the `loop` service in the fabric (if it's not already there):

```
$ ziti-fabric create service loop tcp:127.0.0.1:8171 003
```

Launch a `loop2` listener (in $GOPATH):

```
$ ziti-fabric-test loop2 listener
```
    
Launch a `loop2` dialer (begin generating load):

```
$ ziti-fabric-test loop2 dialer src/github.com/netfoundry/ziti-fabric/fabric/etc/loop2/10-ambient.loop2.yml
```
    
Take a look at the various `loop2` scenario configurations in `etc/loop2` for examples illustrating different workloads.
