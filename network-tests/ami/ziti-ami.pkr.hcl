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

  provisioner "shell" {
    inline = [
# Set custom files with proper permissions
      "sudo chown root /home/ubuntu/99remote-not-fancy",
      "sudo chown :root /home/ubuntu/99remote-not-fancy",
      "sudo chown root /home/ubuntu/51-network-tuning.conf",
      "sudo chown :root /home/ubuntu/51-network-tuning.conf",
      "sudo chown root /home/ubuntu/10-ziti-logs.conf",
      "sudo chown :root /home/ubuntu/10-ziti-logs.conf",



# Move custom files into proper locations
      "sudo mv /home/ubuntu/99remote-not-fancy /etc/apt/apt.conf.d/",
      "sudo mv /home/ubuntu/51-network-tuning.conf /etc/sysctl.d/",
      "sudo mv /home/ubuntu/10-ziti-logs.conf /etc/sysctl.d/",


      "cloud-init status --wait",

      # Add metricsbeat sources
      "curl --fail --silent --show-error --location https://artifacts.elastic.co/GPG-KEY-elasticsearch | gpg --dearmor | sudo dd of=/usr/share/keyrings/elasticsearch-archive-keyring.gpg",
      "echo \"deb [arch=amd64 signed-by=/usr/share/keyrings/elasticsearch-archive-keyring.gpg] https://artifacts.elastic.co/packages/8.x/apt stable main\" | sudo tee -a /etc/apt/sources.list.d/elastic-8.x.list",

      # Install filebeat
      "curl -L -O https://artifacts.elastic.co/downloads/beats/filebeat/filebeat-8.6.2-amd64.deb",
      "sudo dpkg -i filebeat-8.6.2-amd64.deb",

      # add consul sources
      "curl --fail --silent --show-error --location https://apt.releases.hashicorp.com/gpg | gpg --dearmor | sudo dd of=/usr/share/keyrings/hashicorp-archive-keyring.gpg",
      "echo \"deb [arch=amd64 signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main\" | sudo tee -a /etc/apt/sources.list.d/hashicorp.list",

      # Set New Beats files with proper permissions
      "sudo chown root /home/ubuntu/system.yml",
      "sudo chown :root /home/ubuntu/system.yml",
      "sudo chown root /home/ubuntu/filebeat.yml",
      "sudo chown :root /home/ubuntu/filebeat.yml",
      "sudo chown root /home/ubuntu/metricbeat.yml",
      "sudo chown :root /home/ubuntu/metricbeat.yml",

      "sudo filebeat modules enable system",
      "sudo mv /home/ubuntu/filebeat.yml /etc/filebeat/",
      "sudo mv  /home/ubuntu/system.yml /etc/filebeat/modules.d/system.yml",
      "sudo apt update",
      "sudo apt upgrade -y",
      "sudo apt install -y iperf3 tcpdump sysstat",
      "sudo apt install -y metricbeat=8.3.2",
      "sudo mv /home/ubuntu/metricbeat.yml /etc/metricbeat/",
      "sudo metricbeat modules enable aws",
      "sudo apt install -y consul",
      "sudo bash -c \"echo 'ubuntu soft nofile 40960' >> /etc/security/limits.conf\"",
      "sudo sed -i 's/ENABLED=\"false\"/ENABLED=\"true\"/g' /etc/default/sysstat",
    ]
  }
}
