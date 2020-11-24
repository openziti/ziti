#!/dev/null
#
# blame(ken)
#
# source this after sourcing env.sh to append required domain names to the dnsmasq.conf in the same dir which is mounted on dnsmasq server by docker-compose.yml

# DIRNAME defined by init.sh before sourcing this because it's problematic to determine the value of the sourced file in a shell-portable manner
[[ ! -z ${DIRNAME:-} ]] || {
    echo "ERROR: not configuring dnsmasq because DIRNAME is not defined." >&2
}

# expected in same directory as docker-compose.yml
[[ -s ${DIRNAME}/docker-compose.yml ]] || {
    echo "ERROR: missing ${DIRNAME}/docker-compose"
}

DNSCONF=${DIRNAME}/dnsmasq.conf
[[ -s ${DNSCONF} ]] && {
    echo "WARN: clobbered ${DNSCONF}" >&2
} || {
    echo "INFO: created ${DNSCONF}"
}

cat <<EOF >| ${DNSCONF}
port=53
listen-address=${NAMESERVER1:-127.0.0.123}
bind-interfaces
#log all dns queries
log-queries
#don't use host's nameservers in /etc/resolv.conf, only use upstreams define in this conf
no-resolv
# don't forward plain names, only FQDNs
domain-needed
#use google as default nameservers
server=1.1.1.1
server=8.8.8.8
#explicitly defines host-ip mappings appended by init.sh
EOF

for DOMAIN_NAME in ${ZITI_CONTROLLER_HOSTNAME} ${ZITI_EDGE_HOSTNAME} ${ZITI_ZAC_HOSTNAME} ${ZITI_EDGE_ROUTER_HOSTNAME} ${ZITI_EDGE_WSS_ROUTER_HOSTNAME} ${ZITI_ROUTER_BR_HOSTNAME} ${ZITI_ROUTER_BLUE_HOSTNAME} ${ZITI_ROUTER_RED_HOSTNAME}; do
    echo "address=/${DOMAIN_NAME}/127.0.0.1" >> ${DNSCONF}
done

docker-compose --file ${DIRNAME}/docker-compose.yml up --detach dns