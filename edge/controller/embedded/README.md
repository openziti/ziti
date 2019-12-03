This project uses pakr (https://github.com/gobuffalo/packr) to embed the JSON
schema file within the resulting binaries. Working with these files requires
the installation of the pakr utility command.

- go get -u github.com/gobuffalo/packr/packr

The entire, edge/embedded directory structure is embedded into the controller
binary by default.


To ensure that no extra steps are needed for a consumer of this source code,
"Building a Binary (the hard way)" as documented in the packr documentation
is used. This means that the author of any files in the /embedded directory
must run the packr utility command after altering/adding/removing files.

To run the packr utility, navigate to the /ziti project root and execute:

> bin/packr -z

It should scan the source and repack any files. "*-packr.go" files will be
generated. These files SHOULD be checked into source (against the packr
recommendations). This alleviates consumers from having to install and run
the packr utility.