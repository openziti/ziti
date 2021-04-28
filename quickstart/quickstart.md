# Quickstart

This directory contains a set of scripts designed to make it easy to establish a starter overlay network.
The expectation is that these scripts and docker image are useful for learning or for establishing 
simple networks. As with any solution it is common for additional changes to the configuration to be required
after expanding beyond the initial setup.

There are three different modes contained in these folders. One mode allows you very quickly get setup and
run the two main components  of a Ziti network: ziti-controller and ziti-router. The [Express](#express)
configuration will guide you here.

The remaining two modes all use [docker](https://docs.docker.com/get-started/) to establish environments.
The first of the docker-based quickstarts uses [docker-compose](https://docs.docker.com/compose/). 
You will find a fully defined Ziti Network in a compose file which should allow you to understand better
and learn how multiple routers can be linked to form a mesh network or serve as an initial
template to build your own compose file from.

Lastly, you can choose to run [docker](https://docs.docker.com/get-started/) directly. This mode is necessarily
more verbose but should you prefer to not use docker-compose it can also illustrate how to establish
a Ziti Network piece by piece.

## Prerequisties

### Bash

All of these quickstarts will use bash. On MacOS/linux this will be natural however on Windows you'll want
to ensure you have a suitable shell. There are numerous shells available but perhaps the simplest will be
to use [Windows Subsystem for Linux (WSL)](https://docs.microsoft.com/en-us/windows/wsl/install-win10). You 
might also use git-bash, cygwin, or any other bash shell you fancy.

### Docker/Docker Compose

If you are interested in using the quickstarts which use docker/docker-compose you will clearly need to
have one or both installed and be moderately familiar with whichever you are using.

### Review All Scripts

Remember - it's always a good idea to review any scripts before you run them. We encourage you to review
the scripts in these folders before running them.

## Express

By far the easiest way to establish an environment quickly is to simply run the express install script
found at [./quickstart/local/express/express-dev-env.sh](). 

### What It Does

The express install script will do quite a few things to get you bootstrapped.  It will:

1. create a full suite of configuration files located by default at ~/.ziti/quickstart/$(hostname)
    1. create a full suite of PKI
    1. create a config file for the controller
    1. create a config file for an edge router
1. download the latest distribution of ziti from github.com/openziti/ziti/releases
1. unzip the distribution
1. start the `ziti-controller` and `ziti-router` executables
1. the `ziti-controller` should now be exposed on https://$(hostname):1280

## Docker - Compose

The [docker-compose](https://docs.docker.com/compose/) based example will create numerous `ziti-router`s 
as well as spooling up a `ziti-controller` and expose the controller on port 1280.