#!/bin/sh -i

create_client_key_pair(){
    echo "hi there"
    echo "aaaaaaaaaaaaa 1: $1 2: $2 3: $3"

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

echo "Enter the name of the ca (ca.external, ca.internal, ca.3rdparty, ca.ecc, etc.), followed by [enter]"
read cnfname

echo "Enter the name of the key pair files, followed by [enter]"
read kpname

create_client_key_pair $cnfname 1234 $kpname