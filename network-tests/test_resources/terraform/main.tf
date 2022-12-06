variable "environment_tag"    { default = "{{ .Model.MustVariable "environment" }}" }
variable "aws_access_key"     { default = "{{ .Model.MustVariable "credentials.aws.access_key" }}" }
variable "aws_secret_key"     { default = "{{ .Model.MustVariable "credentials.aws.secret_key" }}" }
variable "aws_key_name"       { default = "{{ .Model.MustVariable "credentials.aws.ssh_key_name" }}" }
variable "aws_key_path"       { default = "{{ .Model.MustVariable "credentials.ssh.key_path" }}" }

variable "vpc_cidr"           { default = "10.0.0.0/16" }
variable "public_cidr"        { default = "10.0.0.0/24" }

{{ range $regionId, $region := .Model.Regions }}
module "{{ $regionId }}_region" {
  source          = "{{ $.TerraformLib }}/vpc"
  access_key      = var.aws_access_key
  secret_key      = var.aws_secret_key
  region          = "{{ $region.Region }}"
  vpc_cidr        = var.vpc_cidr
  public_cidr     = var.public_cidr
  az              = "{{ $region.Site }}"
  environment_tag = var.environment_tag
}

{{ range $hostId, $host := $region.Hosts }}
module "{{ $regionId }}_host_{{ $hostId }}" {
  source            = "{{ $.TerraformLib }}/{{ instanceTemplate $host }}_instance"
  access_key        = var.aws_access_key
  secret_key        = var.aws_secret_key
  environment_tag   = var.environment_tag
  instance_type     = "{{ $host.InstanceType }}"
  key_name          = var.aws_key_name
  key_path          = var.aws_key_path
  region            = "{{ $region.Region }}"
  security_group_id = module.{{ $regionId }}_region.security_group_id
  subnet_id         = module.{{ $regionId }}_region.subnet_id
  spot_price        = "{{ $host.SpotPrice }}"
  spot_type         = "{{ $host.SpotType }}"
  name              = "{{ $region.Model.MustVariable "environment" }}.{{ $host.Id }}"
}

output "{{ $regionId }}_host_{{ $hostId }}_public_ip" { value = module.{{ $regionId }}_host_{{ $hostId }}.public_ip }
output "{{ $regionId }}_host_{{ $hostId }}_private_ip" { value = module.{{ $regionId }}_host_{{ $hostId }}.private_ip }
{{ end }}
{{ end }}
