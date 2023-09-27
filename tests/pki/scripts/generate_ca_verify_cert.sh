#!/bin/bash -i

name="temp.validate"

# echo "Enter the path to the CA cert, followed by [enter]:"
# read -e caCertPath

# echo "Enter the path to the CA private key, followed by [enter]:"
# read -e caKeyPath

echo "Enter the path to the CA cnf file, followed by [enter]"
read -e cnfFile

echo "Common Name / Registration Key, followed by [enter]:"
read commonName

openssl genrsa -out $name.key.pem 2048 
openssl req -new -key $name.key.pem -out $name.csr -config $cnfFile -subj '/CN='$commonName'/O=Some Org/C=US'

openssl ca -batch -config $cnfFile -extensions usr_cert -days 1 -in $name.csr -out $name.cert.pem 

cat $name.cert.pem

rm $name.cert.pem
rm $name.csr
rm $name.key.pem