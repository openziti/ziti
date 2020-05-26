# Using The Fabric (JSON) Gateway

Launch the `ziti-fabric-gw` like this:

    $ bin/ziti-fabric-gw run src/github.com/openziti/fabric/fabric/etc/gw.yml

Client access like this:

    $ cd ${GOPATH}/src/bitbucket.org/netfoundry/ziti
    $ curl --cacert fabric/etc/ca/intermediate/certs/ca-chain.cert.pem --cert fabric/etc/ca/intermediate/certs/ctrl-client.cert.pem --key fabric/etc/ca/intermediate/private/ctrl.key.pem https://127.0.0.1:10080/ctrl/services | jq
      % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                     Dload  Upload   Total   Spent    Left  Speed
    100   443  100   443    0     0  88600      0 --:--:-- --:--:-- --:--:-- 88600
    {
      "data": [
        {
          "id": "google",
          "endpointAddress": "tls:www.google.com:443",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "googles",
          "endpointAddress": "tcp:www.google.com:443",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "loop",
          "endpointAddress": "tcp:127.0.0.1:8171",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "loop-broken",
          "endpointAddress": "tcp:127.0.0.1:8171",
          "egress": "003",
          "binding": ""
        },
        {
          "id": "quigley",
          "endpointAddress": "tcp:one.quigley.com:80",
          "egress": "003",
          "binding": ""
        }
      ]
    }

To use `ziti-fabric-gw` with a plain HTTP listener (without TLS), comment out the `certPath` and `keyPath` lines in `gw.yml`:

    # Comment out the following lines to disable the TLS listener
    #certPath:         src/github.com/openziti/fabric/fabric/etc/ca/intermediate/certs/mgmt-gw.cert.pem
    #keyPath:          src/github.com/openziti/fabric/fabric/etc/ca/intermediate/private/mgmt-gw.key.pem

And then the listener can be accessed with plain HTTP:

    $ curl http://127.0.0.1:10080/ctrl/services | jq
      % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                     Dload  Upload   Total   Spent    Left  Speed
    100   443  100   443    0     0   432k      0 --:--:-- --:--:-- --:--:--  432k
    {
      "data": [
        {
          "id": "google",
          "binding": "",
          "endpointAddress": "tls:www.google.com:443",
          "egress": "003"
        }
      ]
    }
