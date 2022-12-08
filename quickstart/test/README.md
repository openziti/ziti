# Quickstart Tests

## Setup
### test.env
This file must be updated with the values specific to the environment you are 
testing. The values will be loaded at test time.

### Testing Docker Environments
There is a value in `test.env` for the docker container name. This is the name 
of the container from which the quickstart files will be read and tested. This 
container name should be the name of a container that has access to all quickstart 
files.

If the container name variable is left blank, the tests will assume the files it 
needs will be on the local machine. The docker container name environment variable 
merely allows the test to `docker cp` the file so it can then be analyzed.

### Testing Remote Environments
Due to the somewhat complex task of copying files from a remote host to analyze them, 
if you are testing a remote environment it is recommended to copy the following files 
to your local machine and provide the local paths to the files in the `test.env` file.

* Ziti Environment Variables file (ex. ziti.env)
* ... Add more files here as tests are updated

## Simple Server test
The simple server test assumes you have a locally accessable server to bind an edge 
router to. If you do not have one, an easy way to set one up is to use
```shell
python3 -m http.server
```
In the case above, the server will be at localhost:8000 so simply update the test.env 
file accordingly.

## Running the tests
To run all of the quickstart tests, simply execute the following command while in 
the `./quickstart/docker/test` directory.
```shell
go test
```
