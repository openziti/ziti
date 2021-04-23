#!/bin/bash
. /openziti/ziti.env

"${HOME}/create-pki.sh"

# create pki

# generates the config file for the controller
"${HOME}/create-controller-config.sh"

# initialize the database with the admin user:
ziti-controller edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p $ZITI_HOME/db

ziti-controller run "${ZITI_HOME}/controller.yaml"
