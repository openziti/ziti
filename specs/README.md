# Edge Specs

This folder contains the specifications for the two current edge APIs: client, management. 

## Updating a Spec

If a spec has a bug and you are changing a spec please do not change these files. These 
files are generated and should not be modified.:

* ./management.yml
* ./client.yml

The actual specs to modify are located in the `./source/` folder

## Updating Client/Server Stubs

After modifying a file, you will need to regenerate the stub code.

* Download/install `swagger` from https://github.com/go-swagger/go-swagger/releases
* cd to the directory containing this README
* issue either:
** `./scripts/generate_rest.sh`
** `powershell -file "./scripts/generate_rest.ps1"`
