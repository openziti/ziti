datacenter = "ziti-build-metrics"
data_dir = "/opt/consul"
encrypt = "${encryption_key}"
advertise_addr="${public_ip}"


tls {
        defaults {
                verify_incoming = false
                verify_outgoing = true

                ca_file="consul/consul-agent-ca.pem"
        }
}

auto_encrypt {
        tls = true
}

acl {
        enabled = true
        default_policy = "allow"
        enable_token_persistence = true
}
