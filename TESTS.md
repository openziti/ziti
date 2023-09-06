# Smoketest

The ziti project uses a fablab based environment to run smoketests. These tests are run
in github, but can be run locally as well.

## Running smoketests locally

First you must have an AWS account with permissions to create VPCs, setup security groups
and run EC2 instances. If you are able to the run the AWS CLI using an access key and 
secret token, the smoketest should work as well.

The following steps assume you are starting at the root of a cloned `ziti` repository.

```
$ cd zititest
$ go install ./...
$ simple-transfer create
$ ZITI_VERSION=0.0.0 simple-transfer up
$ go test -v ./tests/...
$ simple-transfer dispose
```

The HA smoketest is very similar. There's one additional flag needed when creating the instance.

```
$ simple-transfer create -l ha=true
```

So the full set of steps for running the HA smoketest locally are:

```
$ cd zititest
$ go install ./...
$ simple-transfer create -l ha=true
$ ZITI_VERSION=0.0.0 simple-transfer up
$ go test -v ./tests/...
$ simple-transfer dispose
```

## Debugging smoketests

When a smoketest runs in Github, the fablab instance data is uploaded to s3. This includes 
everything need to work with the instance. If the smoketest run fails, the teardown will
be delayed by half of an hour. In that half of an hour the instance data can be downloaded
and the instance can be investigated. If you wish to prevent the instance from being
torn down by the workflow, you can delete the instance data out of S3 and take responsiblility
for cleaning it up yourself.

The instance data is encrypted and requires a passphrase to decrypt. Here's an example 
script which can import a GH actions workflow instance into your local environment.

```
aws s3 cp s3://ziti-smoketest-fablab-instances/simple-transfer-$1.tar.gz.gpg ${HOME}/Downloads/simple-transfer-$1.tar.gz.gpg
FABLAB_PASSPHRASE=<passphrase goes here> simple-transfer import ${HOME}/Downloads/simple-transfer-$1.tar.gz.gpg
rm ${HOME}/Downloads/simple-transfer-$1.tar.gz.gpg
```
You would pass the workflow run number into the script. If you need the GPG passphrase, please ask.

To delete the instance data out of S3, you would run:

```
aws s3 rm s3://ziti-smoketest-fablab-instances/simple-transfer-$1.tar.gz.gpg
```

The HA smoketest is very similar. 

```
aws s3 cp s3://ziti-smoketest-fablab-instances/simple-transfer-ha-$1.tar.gz.gpg ${HOME}/Downloads/simple-transfer-ha-$1.tar.gz.gpg
FABLAB_PASSPHRASE=<passphrase goes here> simple-transfer import ${HOME}/Downloads/simple-transfer-ha-$1.tar.gz.gpg
rm ${HOME}/Downloads/simple-transfer-ha-$1.tar.gz.gpg
```
You would pass the workflow run number into the script. If you need the GPG passphrase, please ask.

To delete the instance data out of S3, you would run:

```
aws s3 rm s3://ziti-smoketest-fablab-instances/simple-transfer-ha-$1.tar.gz.gpg
```


