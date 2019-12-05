# Creating Keys and Certificates for Development

This is not necessary if you are using the PKI infrastructure that is already configured in the development config files (above).

This is only necessary if you want to generate your own PKI infrastructure for your own configuration.


## Create CA and Intermediate

``` 
$ ziti pki create ca --pki-root=~/pki --ca-name=root-ca --ca-file=root-ca
$ ziti pki create intermediate --pki-root=~/pki --ca-name=root-ca
```

## Create Controller Key and Certificates

```
$ ziti pki create key --pki-root=~/pki --ca-name=intermediate --key-file ctrl
$ ziti pki create server --pki-root=~/pki --ca-name=intermediate --server-file=ctrl-server --ip 127.0.0.1 --key-file ctrl
$ ziti pki create client --pki-root=~/pki --ca-name=intermediate --client-file=ctrl-client --key-file ctrl

```

## Create dotziti Key and Certificates

```
$ ziti pki create key --pki-root=~/pki --ca-name=intermediate --key-file dotziti
$ ziti pki create server --pki-root=~/pki --ca-name=intermediate --server-file=dotzeet-server --ip 127.0.0.1 --key-file dotziti
$ ziti pki create client --pki-root=~/pki --ca-name=intermediate --client-file=dotzeet-client --key-file dotziti

```

## Create Router Key and Certificates

This process can be repeated to create development certificates for any number of routers.

```
$ ziti pki create key --pki-root=~/pki --ca-name=intermediate --key-file 002
$ ziti pki create server --pki-root=~/pki --ca-name=intermediate --server-file=002-server --ip 127.0.0.1 --key-file 002 --server-name 002
$ ziti pki create client --pki-root=~/pki --ca-name=intermediate --client-file=002-client --key-file 002 --client-name 002
```