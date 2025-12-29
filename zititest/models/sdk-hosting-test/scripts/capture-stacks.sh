#!/bin/bash
while true; do
  ziti agent list | grep -o -e 'host[a-zA-z0-9\-]*' | xargs -I{} sh -c 'ziti agent stack --app-alias $1 > $1.$(date +%s).stack' -- {}
  sleep 15
done
