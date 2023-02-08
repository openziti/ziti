# Overview

This local development README guides you to install the Ziti stack built from the currently checked out source branch in this repo without any downloads, containers, scripts, or magic.

## Minimum Go Version

You will need to [install a version of Go](https://go.dev/) that is as recent as the version used by this project. Find the current minimum version by running this command to inspect `go.mod`.

```bash
grep -Po '^go\s+\K\d+\.\d+(\.\d+)?$' go.mod
```

## Build and Install All Applications

This repo contains several Go applications, e.g. ziti-controller, ziti-router. These applications were reorganized as subcommands of the `ziti` CLI application, so it's no longer necessary to build the discrete binaries. Only the `ziti` CLI is necessary. The easiest way to build and install `ziti` in `${GOPATH}/bin` is:

```bash
# build and install ziti CLI
go install ./ziti
```

If you add `${GOPATH}/bin` to your executable search `${PATH}` then you may immediately run the newly-built binaries. For example,

```bash
$ ziti version
NAME             VERSION
ziti             v0.0.0
ziti-controller  not installed
ziti-prox-c      not installed
ziti-router      not installed
ziti-tunnel      not installed
ziti-edge-tunnel v0.20.18-local
```

## Build ziti CLI

```bash
# build the binary without installing in GOPATH
go build -o ./build/ziti ./ziti/
# execute the binary
./build/ziti version
```

## Run a Local Ziti Stack

Let's get a local Ziti stack up and running now that you have built and installed all the Ziti apps in this repo.

### Initialize the Go Workspace

This is optional. You may skip initializing a Go workspace if you use release builds from other Go modules that this project depends upon e.g. `openziti/edge`, `openziti/fabric`.

A Go workspace is the best way to use the checked out copy of other modules' source instead of downloading releases from GitHub.

For each Ziti module you wish to develop (modify) you will need to add it to your workspace.

For this example, we'll only add the `edge` module.

```bash
go work init
go work use .
go work use ../edge  # assumes openziti/edge is checked out in sibling dir "edge"
```

This produces a `go.work` file.

```bash
$ cat go.work
go 1.19

use (
    .
    ../edge
)
```

You will need to be aware of the checked out revision in each module because it is used to satisfy imports at a higher precedence than pinned versions in `go.mod`. That is, you may change the version of `edge` that is imported by this module at build time by checking out a different version in the adjacent `edge` repo.

## Further Exploration

Continue your OpenZiti exploration in [the next article about running an OpenZiti stack locally for development](./003-local-deploy.md).
