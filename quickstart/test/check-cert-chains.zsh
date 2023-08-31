#!/usr/bin/env zsh
set -euo pipefail

backup_file(){

  local FILE="$1"
  local BACKUP_FILE; BACKUP_FILE="${FILE}.${DATESTAMP}.bak"
  if [[ -f "$FILE" ]]; then
    echo "INFO: backing up $FILE to $BACKUP_FILE"
    cp "$FILE" "$BACKUP_FILE"
  else
    echo "ERROR: $FILE not found" >&2
    return 1
  fi

}

patch_quickstart(){

  backup_file "${ZITI_PKI_CTRL_CA}"
  # * controller (ctrl plane) intermediate CA cert
  # * controller (ctrl plane) root CA cert
  # * edge controller intermediate CA cert (web PKI)
  # * edge controller root CA cert (web PKI)
  # * edge enrollment signer CA cert
  # * spurious (grandparent) intermediate CA cert
  cat "${ZITI_PKI}/${ZITI_PKI_CTRL_INTERMEDIATE_NAME}/certs/${ZITI_PKI_CTRL_INTERMEDIATE_NAME}.cert" \
      "${ZITI_PKI}/${ZITI_PKI_CTRL_ROOTCA_NAME}/certs/${ZITI_PKI_CTRL_ROOTCA_NAME}.cert" \
      "${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}/certs/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}.cert" \
      "${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}/certs/${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}.cert" \
      "${ZITI_PKI}/${ZITI_PKI_SIGNER_ROOTCA_NAME}/certs/${ZITI_PKI_SIGNER_ROOTCA_NAME}.cert" \
      "${ZITI_PKI}/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}/certs/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}.cert" \
      "${ZITI_PKI}/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}_spurious_intermediate/certs/${ZITI_PKI_SIGNER_INTERMEDIATE_NAME}_spurious_intermediate.cert" \
  > "${ZITI_PKI_CTRL_CA}"  # not "${ZITI_PKI_SIGNER_CERT}" because signerCert.cert no longer aggregated!?
  
  backup_file "${ZITI_HOME}/${ZITI_CTRL_NAME:-${ZITI_NETWORK}}.yaml"
  # Configure the client API web listener's CA cert as the controller edge PKI root CA instead of the intermediate CA
  sed -Ei \
    "s|${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}/certs/${ZITI_PKI_CTRL_EDGE_INTERMEDIATE_NAME}.cert|${ZITI_PKI}/${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}/certs/${ZITI_PKI_CTRL_EDGE_ROOTCA_NAME}.cert|g" \
    "${ZITI_HOME}/${ZITI_CTRL_NAME:-${ZITI_NETWORK}}.yaml"

  echo -e "The following changes were made:"\
          "\n\t* Rebuilt the controller CA bundle"\
          "\n\t* Replaced the controller edge intermediate CA cert in the client API's web listener identity.ca with the controller edge root CA cert"\
          "\n\nPlease restart the controller and the main router and re-run this script without --patch."

}

well_known(){
  if ! (( $# )); then
    echo "ERROR: need exactly one controller edge API to fetch trust anchors, e.g., well_known address:port" >&2
    return 1
  fi
  # download the controller's trust anchor bundle as currently presented to enrollees
  {
    curl -sSkf "https://${1}/.well-known/est/cacerts" \
    | openssl base64 -d \
    | openssl pkcs7 -inform DER -outform PEM -print_certs;
  } 2>/dev/null
}

verify_chain(){
  # verify the leaf cert at index 0 with untrusted intermediates from indices 1 through n-1 and the anchor at index n
  openssl verify \
    -CAfile "${TRUST_ANCHORS}" \
    -untrusted "${1}" \
    "${1}"
}

ctrl_plane(){
  openssl s_client \
    -connect "${ZITI_CTRL_ADVERTISED_ADDRESS}:${ZITI_CTRL_ADVERTISED_PORT}" \
    -servername "${ZITI_CTRL_ADVERTISED_ADDRESS}" \
    -alpn "ziti-ctrl,h2,http/1.1" \
    -showcerts </dev/null
}

edge_web(){
  openssl s_client \
    -connect "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT}" \
    -servername "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}" \
    -showcerts </dev/null
}

public_router(){
  openssl s_client \
    -connect "${ZITI_ROUTER_NAME}:${ZITI_ROUTER_PORT}" \
    -servername "${ZITI_ROUTER_NAME}" \
    -alpn "ziti-edge,h2,http/1.1" \
    -showcerts </dev/null
}

connect_showcerts(){
  openssl s_client \
    -connect "${1}" \
    -showcerts </dev/null
}

diagnose_issuers() {
  if ! [[ $# -eq 2 ]]; then
    echo "ERROR: diagnose_issuers requires exactly two arguments: check_name and chain_file" >&2
    return 1
  fi
  local \
    CHECK_NAME="$1" \
    CHAIN_FILE="$2"
  echo "INFO: $CHECK_NAME issuers:"
  openssl storeutl -certs -noout -text "$CHAIN_FILE" 2>/dev/null \
    | grep -o 'Issuer:.*CN=.*' | while read; do printf "\t%s\n" "$REPLY"; done | uniq
  echo "INFO: Well-known Trust Anchor Subjects must contain the last issuer:"
  openssl storeutl -certs -noout -text "${TRUST_ANCHORS}" 2>/dev/null \
    | grep -o 'Subject:.*CN=.*' | while read; do printf "\t%s\n" "$REPLY"; done | sort -u
}

check_connection(){
  if [[ $# -eq 1 ]]; then
    local CONN="$1"
    local CONN_FILE
    CONN_FILE="${TEMPDIR}/${CONN//:/_}.pem"
  else
    echo "ERROR: need exactly one connection to check, e.g., check_connection address:port" >&2
    return 1
  fi

  local CONNFAIL=0

  [[ -s "${TRUST_ANCHORS}" ]] || {
    if well_known "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT}" > "${TRUST_ANCHORS}"; then
      echo "INFO: downloaded trust anchor bundle from ${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT}"
    elif well_known "${CONN}" > "${TRUST_ANCHORS}"; then
      echo "INFO: downloaded trust anchor bundle from ${CONN}"
    else
      echo  "ERROR: if $CONN does not provide /.well-known/est/cacerts then say --well-known \"EDGE:PORT\" or"\
      "(ZITI_CTRL_EDGE_ADVERTISED_ADDRESS and ZITI_CTRL_EDGE_ADVERTISED_PORT) must be set to fetch trust anchors" >&2
      return 1
    fi
  }
  if connect_showcerts "$CONN" &> "${CONN_FILE}" || true
  if ! grep -qE -- '^-----BEGIN CERTIFICATE-----$' "${CONN_FILE}"; then
    echo "ERROR: failed to fetch certificate chain from ${CONN}" >&2
    return 1
  fi
  if verify_chain "${CONN_FILE}" &>/dev/null; then
    echo -e "\nINFO: $CONN connection verified."
  else
    echo -e "\nERROR: $CONN verifiying connection failed with command:" >&2
    (
      set -x; 
      verify_chain "$CONN_FILE"
    ) >&2 || true
    diagnose_issuers "$CONN" "$CONN_FILE"
    CONNFAIL=1
  fi

  if (( CONNFAIL )); then
    echo -e "\nERROR: At least one connection failed verification. patch the unverified chain(s) manually or add run with"\
    " --patch to try to fix it automatically. You must restart affected services or containers after patch is"\
    " completed before trying again to verify certs." >&2
    return 1
  else
    echo -e "\nINFO: All connections verified."
  fi
}

ziti_env(){
    
  if ! (( $# )); then
    echo "ERROR: no named quickstart targets specified" >&2
    return 1
  fi

  local CHAINFAIL=0

  : "${ZITI_CTRL_ADVERTISED_ADDRESS:=ziti-controller}"
  : "${ZITI_CTRL_ADVERTISED_PORT:=6262}"
  : "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS:=ziti-edge-controller}"
  : "${ZITI_CTRL_EDGE_ADVERTISED_PORT:=1280}"
  : "${ZITI_ROUTER_NAME:=ziti-edge-router}"
  : "${ZITI_ROUTER_PORT:=3022}"

  [[ -s "${TRUST_ANCHORS}" ]] || {
    if ! well_known "${ZITI_CTRL_EDGE_ADVERTISED_ADDRESS}:${ZITI_CTRL_EDGE_ADVERTISED_PORT}" > "${TRUST_ANCHORS}"; then
      echo "ERROR: failed to download the trust anchor bundle. If ZITI_CTRL_EDGE_ADVERTISED_ADDRESS and"\
      "ZITI_CTRL_EDGE_ADVERTISED_PORT are not set then say --well-known 'EDGE:PORT' to fetch trust anchors" >&2
      return 1
    fi
  }

  for CHECK in $@; do
    # fetch the server cert partial chain
    $CHECK > "${CHAINS[$CHECK]}" 2>/dev/null || true
    if verify_chain "${CHAINS[$CHECK]}" &>/dev/null; then
      echo -e "\nINFO: $CHECK certificate chain verified."
    else
      echo -e "\nERROR: $CHECK chain verification failed with command:" >&2
      (
        set -x; 
        verify_chain "${CHAINS[$CHECK]}"
      ) >&2 || true
      diagnose_issuers "$CHECK" "${CHAINS[$CHECK]}"
      CHAINFAIL=1
    fi
  done

    if (( CHAINFAIL )); then
      echo -e "\nERROR: At least one chain failed verification. patch the unverified chain(s) manually or add run with"\
      " --patch to try to fix it automatically. You must restart affected services or containers after patch is"\
      " completed before trying again to verify certs." >&2
      return 1
    else
      echo -e "\nINFO: All named quickstart targets verified."
    fi
}

BASENAME="$(basename "$0")"
DATESTAMP=$(date +%Y%m%d%H%M%S)
TEMPDIR="$(mktemp -d -t "certs.${DATESTAMP}.XXXX")"
TRUST_ANCHORS="${TEMPDIR}/well-known-anchors.pem"
declare -A CHAINS
declare -a CHECK_CHAINS CHECK_CONNECTIONS
CHAINS[ctrl_plane]="${TEMPDIR}/controller-ctrl-chain.pem"
CHAINS[edge_web]="${TEMPDIR}/controller-edge-web-chain.pem"
CHAINS[public_router]="${TEMPDIR}/router-chain.pem"

# echo "DEBUG: \$#=$# \$@=$@"
while (( $# )); do
  case "$1" in
    # space sep list of named quickstart targets, e.g., ctrl_plane uses ZITI_CTRL_ADVERTISED_ADDRESS and
    # ZITI_CTRL_ADVERTISED_PORT
    --check-quickstart)
      shift
      while (( $# )) && [[ ! "$1" =~ ^-- ]]; do
        CHECK_CHAINS+=("$1")
        shift
      done
    ;;
    --patch-quickstart)
      shift
      patch_quickstart
      exit
    ;;
    # space sep list of domain:port pairs to check, e.g., ziti-controller:6262
    --check-conn)
      shift
      while (( $# )) && [[ ! "$1" =~ ^-- ]]; do
        CHECK_CONNECTIONS+=("$1")
        shift
      done
    ;;
    --well-known)
      shift
      if ! well_known "$1" > "${TRUST_ANCHORS}"; then
        echo "ERROR: failed to fetch trust anchors from $1" >&2
        exit 1
      fi
      shift
    ;;
    --help|-h|*)
      echo -e "\nUsage: $BASENAME [ --conn 'CONN:PORT' ... | --well-known 'EDGE:PORT' | --check-quickstart"\
              "TARGET ...| --patch-quickstart | --help ]"\
              "\n\n\t--check-quickstart TARGET ...\tCheck the space-separated named targets using quickstart env vars."\
              "The following quickstart check\n\t\t\t\t\ttargets are available: ${(k)CHAINS[*]}"\
              "\n\n\t--patch-quickstart\t\tUsing quickstart env vars, patch the controller CA bundle and replace the"\
              "controller edge intermediate \n\t\t\t\t\tCA cert in the client API's web listener identity.ca with the"\
              "controller edge root CA cert"\
              "\n\n\t--conn 'CONN1:PORT1' ...\tCheck the specified space-separated list of connections"\
              "\n\t--well-known 'EDGE:PORT'\tOverride quickstart env vars and download the trust anchor bundle from"\
              "the specified controller edge API"\
              "\n\t--help\t\t\t\tShow this help"\
              "\n\n\tIf no options are specified, all named targets are checked using quickstart env vars"
      exit 0
    esac
done

# if no raw connections were specified, check all named qs targets
if (( ! ${#CHECK_CHAINS[@]} )) && ! (( ${#CHECK_CONNECTIONS[@]} )); then
  ziti_env ${(k)CHAINS[@]}
# otherwise, check only the specified named qs targets
else
  for CHECK in ${CHECK_CHAINS[@]}; do
    if [[ -z "${CHAINS[${CHECK}]:-}" ]]; then
      echo "ERROR: ${CHECK} not found, use any of ${(k)CHAINS[*]}" >&2
      exit 1
    fi
  done
fi

if (( ${#CHECK_CONNECTIONS[@]} )); then
  for CONN in ${CHECK_CONNECTIONS[@]}; do
    check_connection "$CONN"
  done
fi

