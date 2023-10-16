# db-creation model

### This model is designed to be used for GitHub Actions to create a test DB and export the pki/identities/DB to s3 buckets for later testing usage. ###

- Only setup for AWS.
- Designed to work with the pete-iperf branch of fablab.
- You will need to supply your own keys/secrets.
- Infrastructure is configured in the main.go in the model.
- This is a very alpha release, minimal features.

### There are several files that will likely need to be customized for your setup: ### 

- ziti/zititest/models/db-creation/main.go - mainly used to alter the model and also your Rsync and Disposal Actions (removing Route 53 A Record)
- ziti/zititest/models/db-creation/actions/bootstrap.go - This is where the meat of the actions take place. Sets up AWS remotely from the GH Runner (using Fablab executable), then runs the DB Creation Script.
- ziti/zititest/models/db-creation/resources/db_creator_script_external.sh - This is the script that interacts with Ziti and creates all the identities, services and policies.
- ziti/zititest/models/db-creation/resources/aws_setup.sh - This will default to us-east-1 region and use JSON output, if you want to change those values do that here.
- ziti/.github/workflows/fablab-db-creation.yml - This is where you will setup your GitHub workflow specifics, inserting your custom secret variable names, etc. As you can see at the end, the following 3 Fablab commands are all that is needed to run this:
    - ```./db-creation create db-creation```
    - ```./db-creation up```
    -  ```./db-creation dispose```

### Once the DB is saved in s3, you will need to pull that and the pki from the proper buckets via the following steps:

#### Non Fablab import (manual) or something designed by you ####
- Make sure AWS CLI is configured on the machine you want the DB imported to.
- cd to the /home/ubuntu/fablab directory which is where the DB lies.
- Stop any existing Ziti processes.
- Simply delete the old DB file or rename it.
- Run the following AWS CLI command to import DB:
    - ```aws s3 cp s3://db-bucket-name/ctrl.db-filename ctrl.db ```
- Remove the contents of the entire pki directory using the following:
    - ```cd pki```
    - ```sudo rm -rf *```
    - ```cd ..```
- Run the following to import the pki directory (replacing pki-s3-bucket-name/pki-folder-name with your names) :
    - ```aws s3 cp --recursive s3://pki-s3-bucket-name/pki-s3-folder-name/ pki/```
- Run the following command while replacing the ziti version number in filename to start the controller:
    - ```nohup  /home/ubuntu/fablab/bin/ziti-v0.28.4 controller run --log-formatter pfxlog /home/ubuntu/fablab/cfg/ctrl.yml --cli-agent-alias ctrl > /home/ubuntu/logs/ctrl.log 2>&1 & ```

#### Fablab import ####
- cd into your local ziti/zititest/models/db-creation/resources folder and then import both the DB and PKI from your s3 buckets:
    - Command to run for your DB import:
        - ```aws s3 cp s3://s3-db-bucket-name/s3-ctrl.db-filename ctrl.db```
    - Commands to run for your PKI import:
        - ```mkdir pki```
        - ```aws s3 cp --recursive s3://pki-s3-bucket-name/pki-s3-folder-name/ pki/```
- Within your main.go for the db-creation model, you should uncomment the 2 following lines within the Distribution portion of the model, around line 123 or so:
    - ```rsync.NewRsyncHost("#ctrl", "resources/ctrl.db", "/home/ubuntu/fablab/ctrl.db"),```
    - ```rsync.NewRsyncHost("#ctrl", "resources/pki/", "/home/ubuntu/fablab/pki/"),```
- Now you should be able to create a fresh db-creation executable by building and run that, which should have the new DB/PKI.