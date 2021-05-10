#!/usr/bin/env bash

function pki_client_server {
  name_local=$1
  ZITI_CA_NAME_local=$2
  ip_local=$3

  if [[ "${ip_local}" == "" ]]; then
    ip_local="127.0.0.1"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${name_local}-server.key"; then
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    ziti pki create server --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --server-file "${name_local}-server" \
          --dns "${name_local},localhost" --ip "${ip_local}" \
          --server-name "${name_local} server certificate"
  else
    echo "Creating server cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    echo "key exists"
  fi

  if ! test -f "${ZITI_PKI}/${ZITI_CA_NAME_local}/keys/${name_local}-client.key"; then
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    ziti pki create client --pki-root="${ZITI_PKI}" --ca-name "${ZITI_CA_NAME_local}" \
          --client-file "${name_local}-client" \
          --key-file "${name_local}-server" \
          --client-name "${name_local}"
  else
    echo "Creating client cert from ca: ${ZITI_CA_NAME_local} for ${name_local}"
    echo "key exists"
  fi
  echo " "
}

function pki_create_ca {
  if ! test -f "${ZITI_PKI}/${1}/keys/${1}.key"; then
    echo "Creating CA: ${1}"
    ziti pki create ca --pki-root="${ZITI_PKI}" --ca-file="${1}" --ca-name="${1} Root CA"
  else
    echo "Creating CA: ${1}"
    echo "key exists"
  fi
  echo " "
}

function pki_create_intermediate {
  if ! test -f "${ZITI_PKI}/${2}/keys/${2}.key"; then
    echo "Creating intermediate: ${1} ${2} ${3}"
    ziti pki create intermediate --pki-root "${ZITI_PKI}" --ca-name "${1}" \
          --intermediate-name "${2}" \
          --intermediate-file "${2}" --max-path-len ${3}
  else
    echo "Creating intermediate: ${1} ${2} ${3}"
    echo "key exists"
  fi
  echo " "
}
function yaml2json()
{
    ruby -ryaml -rjson -e 'puts JSON.pretty_generate(YAML.load(ARGF))' $*
}

function printUsage()
{
    echo "Usage: $1 [cert to test] [ca pool to use]"
}

function verifyCertAgainstPool()
{
    if [[ "" == "$1" ]]
    then
        printUsage "verifyCertAgainstPool"
        return 1
    fi

    if [[ "" == "$2" ]]
    then
        printUsage "verifyCertAgainstPool"
        return 1
    fi

    echo "    Verifying that this certificate:"
    echo "        - $1"
    echo "    is valid for this ca pool:"
    echo "        - $2"
    echo ""
    openssl verify -partial_chain -CAfile "$2" "$1"
    if [ $? -eq 0 ]; then
        echo ""
        echo "============      SUCCESS!      ============"
    else
        echo ""
        echo "============ FAILED TO VALIDATE ============"
    fi
}

function showIssuerAndSubjectForPEM()
{
    echo "Displaying Issuer and Subject for cert pool:"
    echo "    $1"
    openssl crl2pkcs7 -nocrl -certfile $1 | openssl pkcs7 -print_certs -text -noout | grep -E "(Subject|Issuer)"
}
