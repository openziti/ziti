#!/bin/bash
set -e

if [ "$1" == "" ]
then
        echo "please provide the name of the ca already created"
        exit 1
fi;
ca_name=$1

./newid.sh $1
./ott.sh $1
./newid-thirdPartyOTT.sh $1
