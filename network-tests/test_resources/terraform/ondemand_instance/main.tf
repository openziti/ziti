variable "name" {}
variable "access_key" {}
variable "secret_key" {}
variable "environment_tag" { default = "" }
variable "instance_type" {}
variable "key_name" {}
variable "key_path" {}
variable "region" {}
variable "security_group_id" {}
variable "ssh_user" { default = "ubuntu" }
variable "subnet_id" {}
variable "spot_price" {}
variable "spot_type" {}

output "public_ip" { value = aws_instance.fablab.public_ip }
output "private_ip" { value = aws_instance.fablab.private_ip }

provider "aws" {
  access_key = var.access_key
  secret_key = var.secret_key
  region     = var.region
}

data "aws_ami" "ami" {
  most_recent = true
  owners      = ["self"]

  filter {
    name   = "name"
    values = ["ziti-tests-*"]
  }
}

resource "aws_instance" "fablab" {
  ami                         = data.aws_ami.ami.id
  instance_type               = var.instance_type
  key_name                    = var.key_name
  vpc_security_group_ids      = [var.security_group_id]
  subnet_id                   = var.subnet_id
  associate_public_ip_address = true

  tags = {
    Name = var.name
  }
}
