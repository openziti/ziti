#!/bin/bash
. ${HOME}/ziti.env

if [ -d "${ZITI_PKI}/${ZITI_CONTROLLER_ROOTCA_NAME}" ]
then
    echo "Reusing existing PKI...."
else
    echo "Generating PKI"
    "${HOME}/create-pki.sh"
fi


# create pki

# generates the config file for the controller
"${HOME}/create-controller-config.sh"

# initialize the database with the admin user:
ziti-controller edge init "${ZITI_HOME}/controller.yaml" -u "${ZITI_USER}" -p "${ZITI_PWD}"

# create a place for the internal db
mkdir -p $ZITI_HOME/db

ziti-controller run "${ZITI_HOME}/controller.yaml"
