packer {
  required_version = ">= 1.6.0"

  required_plugins {
    amazon = {
      version = ">= 1.1.1"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

source "amazon-ebs" "ziti-tests-ubuntu-ami" {
  ami_description = "An Ubuntu AMI that has everything needed for running fablab smoketests."
  ami_name        = "ziti-tests-{{ timestamp }}"
  ami_regions     = ["us-east-1", "us-west-2"]
  instance_type   = "t2.micro"
  region          = "us-east-1"
  source_ami_filter {
    filters = {
      architecture        = "x86_64"
      name                = "ubuntu/images/*/ubuntu-jammy-22.04-amd64-server-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["099720109477"]
  }
  ssh_username = "ubuntu"
}

build {
  sources = ["source.amazon-ebs.ziti-tests-ubuntu-ami"]
# change the source path to match your environment (some directory with minimum permissions, ideally)
  provisioner "file" {
    source      = "/home/padibona/resources/99remote-not-fancy"
    destination = "/home/ubuntu/99remote-not-fancy"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/51-network-tuning.conf"
    destination = "/home/ubuntu/51-network-tuning.conf"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/10-ziti-logs.conf"
    destination = "/home/ubuntu/10-ziti-logs.conf"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/filebeat.yml"
    destination = "/home/ubuntu/filebeat.yml"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/metricbeat.yml"
    destination = "/home/ubuntu/metricbeat.yml"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/system.yml"
    destination = "/home/ubuntu/system.yml"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/aws.yml"
    destination = "/home/ubuntu/aws.yml"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/beats.crt"
    destination = "/home/ubuntu/beats.crt"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/beats.key"
    destination = "/home/ubuntu/beats.key"
  }

  provisioner "file" {
    source      = "/home/padibona/resources/logstashCA.crt"
    destination = "/home/ubuntu/logstashCA.crt"
  }

  provisioner "shell" {
    inline = [
      # Setup UFW to allow for blocking of 80/443 ports for test
      "sudo ufw default allow outgoing",
      "sudo ufw default allow incoming",
      "sudo ufw deny out 80/tcp",
      "sudo ufw deny out 443/tcp",
      "sudo ufw deny in 80/tcp",
      "sudo ufw deny in 443/tcp",

      # Create directories for beats/logstash certs and keys as well as ctrl db file
      "sudo mkdir /etc/pki/",
      "sudo mkdir /etc/pki/beats/",

      # Set custom files with proper permissions/owners/groups
      "sudo chown root /home/ubuntu/99remote-not-fancy",
      "sudo chown :root /home/ubuntu/99remote-not-fancy",
      "sudo chown root /home/ubuntu/51-network-tuning.conf",
      "sudo chown :root /home/ubuntu/51-network-tuning.conf",
      "sudo chown root /home/ubuntu/10-ziti-logs.conf",
      "sudo chown :root /home/ubuntu/10-ziti-logs.conf",
      "sudo chown root /home/ubuntu/beats.crt",
      "sudo chown :root /home/ubuntu/beats.crt",
      "sudo chmod 0644 /home/ubuntu/beats.crt",
      "sudo chown root /home/ubuntu/beats.key",
      "sudo chown :root /home/ubuntu/beats.key",
      "sudo chmod 0400 /home/ubuntu/beats.key",
      "sudo chown root /home/ubuntu/logstashCA.crt",
      "sudo chown :root /home/ubuntu/logstashCA.crt",
      "sudo chmod 0644 /home/ubuntu/logstashCA.crt",

      # Move custom files into proper locations
      "sudo mv /home/ubuntu/99remote-not-fancy /etc/apt/apt.conf.d/",
      "sudo mv /home/ubuntu/51-network-tuning.conf /etc/sysctl.d/",
      "sudo mv /home/ubuntu/10-ziti-logs.conf /etc/sysctl.d/",
      "sudo mv /home/ubuntu/beats.crt /etc/pki/beats/",
      "sudo mv /home/ubuntu/beats.key /etc/pki/beats/",
      "sudo mv /home/ubuntu/logstashCA.crt /etc/pki/beats/",

      "cloud-init status --wait",

      # Linux updates/package installs
      "sudo apt-get -qq -y update",
      "sudo apt-get -qq -y --no-install-recommends install awscli",
      "sudo apt-get -qq -y --no-install-recommends install jq",

      # Install filebeat
      "curl -L -O https://artifacts.elastic.co/downloads/beats/filebeat/filebeat-7.17.5-amd64.deb",
      "sudo dpkg -i -y filebeat-7.17.5-amd64.deb",

      # Install metricbeat
      "curl -L -O https://artifacts.elastic.co/downloads/beats/metricbeat/metricbeat-7.17.5-amd64.deb",
      "sudo dpkg -i -y metricbeat-7.17.5-amd64.deb",

      # add consul sources
      "curl --fail --silent --show-error --location https://apt.releases.hashicorp.com/gpg | gpg --dearmor | sudo dd of=/usr/share/keyrings/hashicorp-archive-keyring.gpg",
      "echo \"deb [arch=amd64 signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main\" | sudo tee -a /etc/apt/sources.list.d/hashicorp.list",

      # Set New Beats files with proper permissions
      "sudo chown root /home/ubuntu/system.yml",
      "sudo chown :root /home/ubuntu/system.yml",
      "sudo chown root /home/ubuntu/aws.yml",
      "sudo chown :root /home/ubuntu/aws.yml",
      "sudo chown root /home/ubuntu/filebeat.yml",
      "sudo chown :root /home/ubuntu/filebeat.yml",
      "sudo chown root /home/ubuntu/metricbeat.yml",
      "sudo chown :root /home/ubuntu/metricbeat.yml",

      "sudo filebeat modules enable system",
      "sudo mv /home/ubuntu/filebeat.yml /etc/filebeat/",
      "sudo mv  /home/ubuntu/system.yml /etc/filebeat/modules.d/system.yml",

      "sudo apt-get -qq -y install iperf3 tcpdump sysstat",
      "sudo mv /home/ubuntu/metricbeat.yml /etc/metricbeat/",
      "sudo apt-get -qq -y install consul",
      "sudo systemctl enable metricbeat",
      "sudo systemctl start metricbeat",
      "sudo systemctl enable filebeat",
      "sudo systemctl start filebeat",

      "sudo bash -c \"echo 'ubuntu soft nofile 40960' >> /etc/security/limits.conf\"",
      "sudo sed -i 's/ENABLED=\"false\"/ENABLED=\"true\"/g' /etc/default/sysstat",
    ]
  }
}
