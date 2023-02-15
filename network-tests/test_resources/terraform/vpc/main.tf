variable "access_key" {}
variable "secret_key" {}
variable "region" {}
variable "vpc_cidr" {}
variable "public_cidr" {}
variable "az" {}
variable "environment_tag" {}

output "security_group_id" { value = aws_security_group.fablab.id }
output "subnet_id" { value = aws_subnet.fablab.id }

provider "aws" {
  access_key = var.access_key
  secret_key = var.secret_key
  region     = var.region
}

resource "aws_vpc" "fablab" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  tags = {
    Name = var.environment_tag
  }
}

resource "aws_internet_gateway" "fablab" {
  vpc_id = aws_vpc.fablab.id
}

resource "aws_subnet" "fablab" {
  vpc_id            = aws_vpc.fablab.id
  cidr_block        = var.public_cidr
  availability_zone = var.az
  tags = {
    Name = var.environment_tag
  }
}

resource "aws_route_table" "fablab" {
  vpc_id = aws_vpc.fablab.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.fablab.id
  }
  tags = {
    Name = var.environment_tag
  }
}

resource "aws_route_table_association" "fablab" {
  subnet_id      = aws_subnet.fablab.id
  route_table_id = aws_route_table.fablab.id
}

resource "aws_security_group" "fablab" {
  name   = var.environment_tag
  vpc_id = aws_vpc.fablab.id

  ingress {
    from_port   = 10000
    to_port     = 10000
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 8171
    to_port     = 8171
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 7001
    to_port     = 7005
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 7001
    to_port     = 7001
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 6262
    to_port     = 6262
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 6262
    to_port     = 6262
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 6000
    to_port     = 6009
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 6000
    to_port     = 6009
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 1280
    to_port     = 1280
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = {
    Name = var.environment_tag
  }
}