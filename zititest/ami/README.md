# Ziti AMI

This folder shows how to use [Packer](https://www.packer.io/) to create an [Amazon Machine
Image (AMI)](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/AMIs.html) that has the necessary prerequisites for running fablab based OpenZiti smoketests installed on top of Ubuntu 22.04.

## Quick start

To build the Ziti AMI:

1. `git clone` this repo to your computer.
1. Install [Packer](https://www.packer.io/).
1. Configure your AWS credentials using one of the [options supported by the AWS
   SDK](http://docs.aws.amazon.com/sdk-for-java/v1/developer-guide/credentials.html). Usually, the easiest option is to
   set the `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` environment variables.
1. Run `packer build ziti-ami.pkr.hcl`.

When the build finishes, it will output the ID of the new AMI.