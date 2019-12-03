#!/bin/bash -i

echo "Enter a name for the output cert, followed by [enter]"
read name

openssl genrsa -out $name.key.pem 2048 
openssl req -new -key $name.key.pem -out $name.csr

echo ""
echo "Outputs:"
echo "    csr: $name.csr"
echo "    key: $name.key.pem"
echo ""
echo "CSR contents:"
echo ""

cat $name.csr
