#!/bin/sh
set -e

fingerprints=""

revoke_cert() {
    openssl ca -config cnf/openssl.$1.cnf -passin pass:$2 -revoke $2 || true
}

create_server_key_pair() {
    openssl ca -config cnf/openssl.$1.cnf -passin pass:$2 -revoke $3.cert.pem || true > /dev/null
    rm $3.cert.pem || true

    echo -e "\e[36mCreating external server certificate for: $3\e[0m"

    openssl ecparam -name secp384r1 -genkey -out $3.key.pem
    openssl req  -batch -config cnf/openssl.$3.cnf -key $3.key.pem -new -sha256 -out $3.csr.pem
    openssl ca -batch -config cnf/openssl.$1.cnf -extensions server_cert -days 375 -notext -md sha256 -in $3.csr.pem -out $3.cert.pem -passin pass:$2 -extfile cnf/openssl.$3.cnf -extensions v3_req
    cat $3.cert.pem  $1/certs/$1.cert.pem > $3.chain.cert.pem
}

create_client_key_pair(){
    openssl ca -config cnf/openssl.$1.cnf -passin pass:$2 -revoke $3.cert.pem || true
    rm $3.cert.pem || true

    echo -e "\e[36mCreating an external client certificate for: $3\e[0m"
    openssl ecparam -name secp384r1 -genkey -out $3.key.pem

    openssl req  -batch -config cnf/openssl.$1.cnf -key $3.key.pem -new -sha256 -out $3.csr.pem
    openssl ca -batch -config cnf/openssl.$1.cnf -extensions usr_cert -days 375 -notext -md sha256 -in $3.csr.pem -out $3.cert.pem -passin pass:$2

    rm $3.csr.pem || true
    newprint="$(openssl x509 -noout -fingerprint -inform pem -in $3.cert.pem)"
    nl=$'\n'
    fingerprints="$fingerprints$nl    - $3: $newprint"
}


create_server_key_pair ca.external 1234  ziti-dev-controller01.external

create_client_key_pair ca.external 1234 client01
create_client_key_pair ca.external 1234 client02
create_client_key_pair ca.external 1234 client03

outputMsg="
--------------------------------------------------------------------------------

Done.

The certs generated include 127.0.0.1 as well as localhost as SANs for all
certificates in addition their their ziti-dev-* and ziti-dev-*.localhost
names for convenience.

To use the custom host names, please add them to your hosts file:

127.0.0.1	ziti-dev-controller01
127.0.0.1	ziti-dev-controller01.localhost



The following client certificates were generated:
$fingerprints

!!! This message has been output to fingerprints.txt for future reference !!!

"

echo "${outputMsg}" | tee fingerprints.txt

echo "Done"