#!/usr/bin/env bash
#
# bootstrap the OpenZiti Controller with PKI, config file, and database
#

set -o errexit
set -o nounset
set -o pipefail

# use the ziti executable that the 'openziti' package installed
PATH=/opt/openziti/bin:$PATH

#
# defaults
#

# used by "ziti pki create server" as DNS SAN and "ziti create config controller" as advertised address
: "${ZITI_CONTROLLER_ADVERTISED_ADDRESS:=$(hostname -f)}"

function makePki() {
  #
  # create root and intermediate CA
  #

  if [ "$ZITI_CA_FILE" == "$ZITI_INTERMEDIATE_FILE" ]; then
    echo "ERROR: ZITI_CA_FILE and ZITI_INTERMEDIATE_FILE must be different" >&2
    exit 1
  fi

  ROOT_CA_DIR="./${ZITI_PKI_ROOT}/${ZITI_CA_FILE}"
  if ! [ -d "$ROOT_CA_DIR" ]; then
    ziti pki create ca \
      --pki-root "./${ZITI_PKI_ROOT}" \
      --ca-file "${ZITI_CA_FILE}"
  fi

  ZITI_PKI_SIGNER_CERT="./${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_INTERMEDIATE_FILE}.cert"
  ZITI_PKI_SIGNER_KEY="./${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_INTERMEDIATE_FILE}.key"
  if [[ ! -s "$ZITI_PKI_SIGNER_CERT" && ! -s "$ZITI_PKI_SIGNER_KEY" ]]; then
    ziti pki create intermediate \
      --pki-root "./${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_CA_FILE}" \
      --intermediate-file "${ZITI_INTERMEDIATE_FILE}"
  elif [[ ! -s "$ZITI_PKI_SIGNER_CERT" || ! -s "$ZITI_PKI_SIGNER_KEY" ]]; then
    echo "ERROR: $ZITI_PKI_SIGNER_CERT and $ZITI_PKI_SIGNER_KEY must both exist or neither exist as non-empty files" >&2
    exit 1
  fi

  #
  # create server and client keys
  #

  if [ "$ZITI_SERVER_FILE" == "$ZITI_CLIENT_FILE" ]; then
    echo "ERROR: ZITI_SERVER_FILE and ZITI_CLIENT_FILE must be different" >&2
    exit 1
  fi

  ZITI_PKI_CTRL_KEY="./${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_SERVER_FILE}.key"
  if ! [ -s "$ZITI_PKI_CTRL_KEY" ]; then
    ziti pki create key \
      --pki-root "./${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}"
  fi

  # use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  # each
  # CLIENT_KEY_FILE="./${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/keys/${ZITI_CLIENT_FILE}.key"
  # if ! [ -s "$CLIENT_KEY_FILE" ]; then
  #   ziti pki create key \
  #     --pki-root "./${ZITI_PKI_ROOT}" \
  #     --ca-name "${ZITI_INTERMEDIATE_FILE}" \
  #     --key-file "${ZITI_CLIENT_FILE}"
  # fi

  #
  # create server and client certs
  #

  # server cert
  ZITI_PKI_CTRL_SERVER_CERT="./${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_SERVER_FILE}.chain.pem"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_SERVER_CERT" ]]; then
    ziti pki create server \
      --pki-root "./${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --server-file "${ZITI_SERVER_FILE}" \
      --dns "${ZITI_CONTROLLER_ADVERTISED_ADDRESS}" \
      --allow-overwrite
  fi

  # client cert
  #   use the server key for both client and server certs until "ziti create config controller" supports separate keys for
  #   each
  ZITI_PKI_CTRL_CERT="./${ZITI_PKI_ROOT}/${ZITI_INTERMEDIATE_FILE}/certs/${ZITI_CLIENT_FILE}.cert"
  if [[ "${ZITI_AUTO_RENEW_CERTS}" == true || ! -s "$ZITI_PKI_CTRL_CERT" ]]; then
    ziti pki create client \
      --pki-root "./${ZITI_PKI_ROOT}" \
      --ca-name "${ZITI_INTERMEDIATE_FILE}" \
      --key-file "${ZITI_SERVER_FILE}" \
      --client-file "${ZITI_CLIENT_FILE}" \
      --allow-overwrite
  fi

}

function makeConfig() {
  #
  # create config file
  #

  # set the path to the root CA cert
  export ZITI_PKI_CTRL_CA="./${ZITI_PKI_ROOT}/${ZITI_CA_FILE}/certs/${ZITI_CA_FILE}.cert"

  # set the interface address on which to listen for connections; e.g., 0.0.0.0
  export ZITI_CTRL_BIND_ADDRESS="${ZITI_CONTROLLER_BIND_ADDRESS}"
  export ZITI_CTRL_EDGE_BIND_ADDRESS="${ZITI_CONTROLLER_BIND_ADDRESS}"

  # set the URI of the router ctrl plane; e.g., ctrl.endpoint: ziti.example.com:443
  export ZITI_CTRL_ADVERTISED_ADDRESS="${ZITI_CONTROLLER_ADVERTISED_ADDRESS}"
  export ZITI_CTRL_ADVERTISED_PORT="${ZITI_CONTROLLER_ADVERTISED_PORT}"

  # set the URI of the edge-client API (uses same TCP port); e.g., ztAPI: ziti.example.com:443
  export ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="${ZITI_CONTROLLER_ADVERTISED_ADDRESS}"
  export ZITI_CTRL_EDGE_ADVERTISED_PORT="${ZITI_CONTROLLER_ADVERTISED_PORT}"

  # export the vars that were assigned inside this script to set the path to the server and client certs and their common
  # private key, and the intermediate (signer) CA cert and key
  export  ZITI_PKI_CTRL_SERVER_CERT \
          ZITI_PKI_CTRL_CERT \
          ZITI_PKI_CTRL_KEY \
          ZITI_PKI_SIGNER_CERT \
          ZITI_PKI_SIGNER_KEY

  if [[ ! -s "./${ZITI_CONTROLLER_CONFIG_FILE}" || "${1:-}" == --force ]]; then
    ziti create config controller \
      --output "./${ZITI_CONTROLLER_CONFIG_FILE}"
  fi

}

function makeDatabase() {

  #
  # create default admin in database
  #

  if [ -s "./${ZITI_CTRL_DATABASE_FILE}" ]; then
    return 0
  fi

  # if the database file is in a subdirectory, create the directory so that "ziti controller edge init" can load the
  # controller config.yml which contains a check to ensure the directory exists
  DB_DIR="$(dirname "${ZITI_CTRL_DATABASE_FILE}")"
  if ! [ "$DB_DIR" == "." ]; then
    mkdir -p "./$DB_DIR"
  fi

  if [[ $(wc -c <<< "${ZITI_PWD:-}") -gt 5 || -s /run/credentials/${UNIT_NAME:=ziti-controller.service}/ZITI_PWD ]]; then
    ziti controller edge init "./${ZITI_CONTROLLER_CONFIG_FILE}" \
      --username "${ZITI_USER}" \
      --password "${ZITI_PWD:-$(< "/run/credentials/${UNIT_NAME}/ZITI_PWD")}"
  else
    echo  "ERROR: need admin password; use LoadCredential or SetCredential in"\
          " /lib/systemd/system/ziti-controller.service or set env var ZITI_PWD with at least 5 characters" >&2
  fi

}

# make PKI unless it exists if true
if [ "${ZITI_BOOTSTRAP_PKI}"      == true ]; then
  makePki
fi

# make config file unless it exists if true, set force to overwrite
if [ "${ZITI_BOOTSTRAP_CONFIG}"   == true ]; then
  makeConfig
elif [ "${ZITI_BOOTSTRAP_CONFIG}" == force ]; then
  makeConfig --force
fi

# make database unless it exists if true
if [ "${ZITI_BOOTSTRAP_DATABASE}" == true ]; then
  makeDatabase
fi
