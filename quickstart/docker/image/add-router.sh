#!/bin/bash

# For edge router setup we will need to address some external variables.
# this section needs to move to doucmentation.
#export EXTERNAL_IP="$(curl -s eth0.me)"       
#export ZITI_EDGE_CONTROLLER_IP_OVERRIDE="127.0.0.1"
#export ZITI_EDGE_ROUTER_IP_OVERRIDE="${EXTERNAL_IP}"
#export ZITI_EDGE_CONTROLLER_PORT=1280
#export ZITI_EDGE_CONTROLLER_MANAGEMENT_PORT=6262
#export ZITI_EDGE_ROUTER_PORT=443
#export ZITI_EDGE_ROUTER_LAN_PORT="ens3"
#export ZITI_EDGE_ROUTER_NAME="ER_NEWNAME"


# For ease of use, we will introduce an optional parameter for controller ip.
if [ $# -eq 0 ]; then
   if [[ "${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}" != "" ]]; then
      controller_ip=${ZITI_EDGE_CONTROLLER_IP_OVERRIDE}
   else
      controller_ip="127.0.0.1"
   fi
elif [ $# -eq 1 ]; then
   # user specified controller ip
   controller_ip=$1
else
   echo ""
   echo "Usage:"
   echo "     $0 <controller-ip>"
   exit
fi

if [[ "${ZITI_EDGE_CONTROLLER_PORT}" != "" ]]; then
   ctrl_port=${ZITI_EDGE_CONTROLLER_PORT}
else
   ctrl_port="1280"
fi

if [[ "${ZITI_EDGE_CONTROLLER_MANAGEMENT_PORT}" != "" ]]; then
   mgmt_port=${ZITI_EDGE_CONTROLLER_MANAGEMENT_PORT}
else
   mgmt_port="6262"
fi

if [[ "${ZITI_EDGE_ROUTER_LAN_PORT}" != "" ]]; then
   lan_port=${ZITI_EDGE_ROUTER_LAN_PORT}
else
   lan_port="ens3"
fi

if [[ "${ZITI_EDGE_ROUTER_PORT}" != "" ]]; then
   rport=${ZITI_EDGE_ROUTER_PORT}
else
   rport="443"
fi

if [[ "${ZITI_EDGE_ROUTER_NAME}" != "" ]]; then
   er_name=${ZITI_EDGE_ROUTER_NAME}
else
   er_name="none"
fi

router_ip=$(ip addr show ${lan_port} | grep "inet\b" | awk '{print $2}' | cut -d/ -f1)

echo Use ${router_ip} as internal IP

cat <<EOT >config.yml
v: 3

identity:
   cert: "certs/identity.cert.pem"
   server_cert: "certs/internal.chain.cert.pem"
   key: "certs/internal.key.pem"
   ca: "certs/intermediate-chain.pem"

edge:
  csr:
    country: US
    locality: Charlotte
    organization: Netfoundry
    organizationalUnit: ADV-DEV
    #province: NC
    sans:
      dns:
        - "localhost"
      ip:
        - "127.0.0.1"
        - "router_ip"
        - "external_router_ip"

ctrl:
  endpoint: tls:edgecontroller:ctrl_port

link:
  dialers:
    - binding: transport

listeners:
  - binding: edge
    address: tls:0.0.0.0:rport
    options:
      advertise: advertise_ip:rport
      maxQueuedConnects:      50
      maxOutstandingConnects: 100
      connectTimeoutMs:       3000
  - binding: tunnel
    options:
      svcPollRate: 15s
      resolver: udp://router_ip:53
      dnsSvcIpRange: 100.64.0.1/10
      lanIf: lan_port
EOT

#create path for router binaries, certs and congig
mkdir -p certs

#update config template with external ip of new router
if [[ "${ZITI_EDGE_ROUTER_IP_OVERRIDE}" != "" ]]; then
   echo ${ZITI_EDGE_ROUTER_IP_OVERRIDE}
   # user specified external ip for router, use that for advertisement
   sed -i "s/external_router_ip/${ZITI_EDGE_ROUTER_IP_OVERRIDE}/" config.yml
   sed -i "s/advertise_ip/${ZITI_EDGE_ROUTER_IP_OVERRIDE}/" config.yml
else
   sed -i "s/advertise_ip/${router_ip}/" config.yml
   sed -i "s/- \"external_router_ip\"//" config.yml
fi

#update config template with private ip of new router
sed -i "s/router_ip/$router_ip/" config.yml

#update config template with public ip of controller
sed -i "s/edgecontroller/$controller_ip/" config.yml

#update config template with edge listening port of new router
sed -i "s/rport/$rport/" config.yml

#update config template with controller fabric listening port
sed -i "s/ctrl_port/$mgmt_port/" config.yml

#update config template with name of lan interface for new router
sed -i "s/lan_port/$lan_port/" config.yml

if [ -f "ziti/ziti" ]
then
   echo "ziti binary found skipping download!"
else
   zitilatest=$(curl -s https://api.github.com/repos/openziti/ziti/releases/latest) 
   version=$(echo "${zitilatest}" | tr '\r\n' ' ' | jq -r '.tag_name' |tr -d v)
      #download ziti
   echo ''
   echo ${BBlue}curl -OL https://github.com/openziti/ziti/releases/download/v${version}/ziti-linux-amd64-${version}.tar.gz${NC}
   curl -OL https://github.com/openziti/ziti/releases/download/v${version}/ziti-linux-amd64-${version}.tar.gz
   #untar ziti
   echo ''
   echo ${BBlue}tar zxvf ziti-linux-amd64-${version}.tar.gz${NC}
   tar zxvf ziti-linux-amd64-${version}.tar.gz
fi

#login to ziti cli
ziti/ziti edge login "${controller_ip}:${ctrl_port}" -u "admin" -p "admin" -y

#create new edge router with integrated tunnle and save enrollment jwt
echo ''
echo ${BBlue}ziti edge create edge-router ${er_name} -t -o enroll.jwt${NC}
ziti/ziti edge create edge-router ${er_name} -t -o enroll.jwt

#enroll edge router with jwt stored in enroll.jwt -- Assumes router config and path for certificates defined are already created
echo ''
echo ${BBlue}sudo ziti/ziti-router enroll config.yml --jwt enroll.jwt${NC}
sudo ziti/ziti-router enroll config.yml --jwt enroll.jwt

#add ziti-resolver to /etc/systemd/resolved.conf required if you want to use router to resolve ziti hostname based services
echo ''
echo ${BBlue}sudo sh -c "echo DNS=${router_ip} >> /etc/systemd/resolved.conf"${NC}
sudo sh -c "echo DNS=${router_ip} >> /etc/systemd/resolved.conf"

#restart systemd-resolved
echo ''
echo ${BBlue}sudo systemctl restart systemd-resolved${NC}
sudo systemctl restart systemd-resolved

echo ''
echo "To start edge router:  sudo ziti/ziti-router run config.yml "
