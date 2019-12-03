ziti edge controller login -u "admin" -p "admin" "${edge_controller_uri}" -c ${ZITI_HOME}/pki/local-root-ca/certs/local-root-ca.cert
svc="wttr.in"
app_wan="appwan-${svc}"

ziti edge controller delete app-wan "${app_wan}"
ziti edge controller delete service "${svc}"
ziti edge controller delete identity "mydevice"

ziti edge controller create identity device mydevice

svc="www.wttr.in"
svc_name="svc_${svc}"
svc_port=80
cluster=$(ziti edge controller list clusters | awk '{$1=$1};1' | cut -d " " -f4)
egress_router=local-fabric-router-red

ziti edge controller login -u "admin" -p "admin" "https://localhost:1280" -c ${ZITI_HOME}/pki/local-root-ca/certs/local-root-ca.cert
ziti edge controller create service "${svc_name}" "${svc}" "${svc_port}" "${egress_router}" "tcp://${svc}:${svc_port}" -c "${cluster}"
ziti edge controller create app-wan "clint-appwan" -s "${svc_name}"

svc="eth0.me"
svc_name="svc_${svc}"
svc_port=80
egress_router=local-fabric-router-blue
ziti edge controller create service "${svc_name}" "${svc}" "${svc_port}" "${egress_router}" "tcp://${svc}:${svc_port}" -c "${cluster}"


svc="www.wttr.in"
svc_name="svctls_${svc}"
svc_port=443
egress_router=local-fabric-router-red
ziti edge controller create service "${svc_name}" "${svc}" "${svc_port}" "${egress_router}" "tcp://${svc}:${svc_port}" -c "${cluster}"

svc="ssh"
svc_name="svctls_${svc}"
svc_port="22"
egress_router=local-fabric-router-blue
ziti edge controller create service "${svc_name}" "192.168.100.100" "${svc_port}" "${egress_router}" "tcp://localhost:${svc_port}" -c "${cluster}"

