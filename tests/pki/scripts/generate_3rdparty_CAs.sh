#!/bin/sh
set -eE


echo ""
echo -e "\e[31mWarning! This will delete all existing *.pem files in this directory structure"
echo "and remove all CA control files! You will have to generate new certs if you"
echo "continue as any other certs signed by any previous CA will become invalid."
echo ""
echo -e "\e[0mPress any key to continue."
read wait

echo -e "\e[36mRemove previous CAs if they exist\e[0m"

rm -rf ca.3rdparty || true

echo -e "\e[36mRemove previous key pairs if they exist\e[0m"

rm *.pem || true


create_ca_dir_structure() {
    echo -e "\e[36mCreate base folders/config for: $1\e[0m"
    mkdir -p $1/certs
    mkdir -p $1/crl
    mkdir -p $1/newcerts
    mkdir -p $1/private
    touch $1/index.txt
    touch $1/index.txt.attr
    echo 00 > $1/crlnumber
    openssl rand -hex 16 > $1/serial
}

create_ca_key_pair() {
    echo -e "\e[36mCreate e$1 private key\e[0m"
    openssl ecparam -name secp384r1 -genkey -out $1/private/$1.key.pem
    openssl ec -in $1/private/$1.key.pem -out $1/private/$1.key.pem -aes256 -passout pass:$2
    echo -e "\e[36mCreate $1 certificate\e[0m"
    openssl req -batch -config cnf/openssl.$1.cnf -key $1/private/$1.key.pem -new -x509 -days 7300 -sha256 -extensions v3_ca -out $1/certs/$1.cert.pem -passin pass:$2
}

create_ca() {
    create_ca_dir_structure $1
    create_ca_key_pair $1 $2
}

create_ca ca.3rdparty 1234

