#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

function checkDns(){
    # only param is an IPv4
    [[ $# -eq 1 && $1 =~ ([0-9]{1,3}\.){3}[0-9]{1,3} ]] || {
        echo "ERROR: need IPv4 of miniziti ingress as only param to checkDns()" >&2
        return 1
    }
    if grep -q "$1.*minicontroller.ziti" /etc/hosts \
        || nslookup minicontroller.ziti | grep -q "$1"; then
        echo "INFO: host DNS found expected minikube ingress IP '$1'"
        return 0
    else
        echo    "ERROR: /etc/hosts does not contain the miniziti record."\
                " Consult docs for Windows (WSL), macOS, and Linux solutions"\
                " before re-running this script."
        return 1
    fi
}

# require commands
declare -a BINS=(jq ziti minikube kubectl helm)
for BIN in "${BINS[@]}"; do
    if ! command -v "$BIN" &>/dev/null; then
        echo "ERROR: this script requires commands '${BINS[*]}'. Please install on the search PATH and try again." >&2
        $BIN || exit 1
    fi
done

# start unless running
echo "INFO: waiting for minikube to be ready"
if ! minikube --profile miniziti status 2>/dev/null | grep -q "apiserver: Running"; then
    minikube --profile miniziti start >/dev/null
fi

MINIKUBE_NODE_EXTERNAL=$(minikube --profile miniziti ip)

if [[ -n "${MINIKUBE_NODE_EXTERNAL:-}" ]]; then
    echo "INFO: the minikube external IP is ${MINIKUBE_NODE_EXTERNAL}"
else
    echo "ERROR: failed to find minikube external IP" >&2
    exit 1
fi

# verify current context can connect to apiserver
kubectl cluster-info >/dev/null

# enable ssl-passthrough for OpenZiti ingresses
if kubectl get deployment "ingress-nginx-controller" \
    --namespace ingress-nginx \
    --output 'go-template={{ (index .spec.template.spec.containers 0).args }}' 2>/dev/null \
    | grep -q enable-ssl-passthrough; then
    echo "INFO: ingress-nginx has ssl-passthrough enabled"
else
    echo "INFO: installing ingress-nginx"
    # enable minikube addons for ingress-nginx
    minikube --profile miniziti addons enable ingress >/dev/null
    minikube --profile miniziti addons enable ingress-dns >/dev/null

    echo "INFO: patching ingress-nginx deployment to enable ssl-passthrough"
    kubectl patch deployment "ingress-nginx-controller" \
        --namespace ingress-nginx \
        --type='json' \
        --patch='[{"op": "add",
            "path": "/spec/template/spec/containers/0/args/-",
            "value":"--enable-ssl-passthrough"
            }]' >/dev/null
fi

echo "INFO: waiting for ingress-nginx to be ready"
# wait for ingress-nginx
kubectl wait jobs "ingress-nginx-admission-patch" \
    --namespace ingress-nginx \
        --for condition=complete \
        --timeout=120s >/dev/null

kubectl wait pods \
    --namespace ingress-nginx \
    --for=condition=ready \
    --selector=app.kubernetes.io/component=controller \
    --timeout=120s >/dev/null

if [[ -n "${DEBUG_WSL:-}" ]] || grep -qi "microsoft" /proc/sys/kernel/osrelease 2>/dev/null; then
    echo "INFO: detected WSL. Privileged minikube tunnel required."\
        "You may be prompted for your WSL user's password to proceed."\
        "minikube tunnel will write log messages in /tmp/minitunnel.log"
    (set -x;
        # ensure stale tunnels are not running
        sudo pkill -f 'minikube --profile miniziti tunnel'
        # this approach allows us to grab stdin and prompt for password before
        # proceeding in the background
        sudo --background bash -c \
            "MINIKUBE_HOME=${MINIKUBE_HOME:-${HOME}/.minikube} "\
            "KUBECONFIG=${KUBECONFIG:-${HOME}/.kube/config} "\
                "minikube --profile miniziti tunnel &>> /tmp/minitunnel.log"
    )
    # recommend /etc/hosts change unless DNS is configured to reach the minikube node IP
    checkDns "127.0.0.1"
else
    if [[ ${OSTYPE:-} =~ [Dd]arwin ]]; then
        # Like WSL, macOS uses localhost port forwarding to reach the minikube node IP
        checkDns "127.0.0.1"
    else
        # Exceptions notwithstanding, Linux, etc. presumably have an IP route to
        # the minikube node IP
        checkDns "$MINIKUBE_NODE_EXTERNAL"
    fi
fi

echo "INFO: applying Custom Resource Definitions: Certificate, Issuer, and Bundle"
kubectl apply \
    --filename https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.crds.yaml >/dev/null
kubectl apply \
    --filename https://raw.githubusercontent.com/cert-manager/trust-manager/v0.4.0/deploy/crds/trust.cert-manager.io_bundles.yaml >/dev/null

if helm repo list | grep -q openziti; then
    echo "INFO: refreshing OpenZiti Helm Charts"
    helm repo update openziti
else
    echo "INFO: subscribing to OpenZiti Helm Charts"
    helm repo add openziti https://docs.openziti.io/helm-charts/
fi

if helm list --namespace ziti-controller --all | grep -q minicontroller; then
    echo "INFO: upgrading OpenZiti Controller, Cert Manager, Trust Manager"
    helm upgrade "minicontroller" openziti/ziti-controller \
        --namespace ziti-controller \
        --set clientApi.advertisedHost="minicontroller.ziti" \
        --values https://docs.openziti.io/helm-charts/charts/ziti-controller/values-ingress-nginx.yaml >/dev/null
else
    echo "INFO: installing OpenZiti Controller, Cert Manager, Trust Manager"
    helm install "minicontroller" openziti/ziti-controller \
        --namespace ziti-controller --create-namespace \
        --set clientApi.advertisedHost="minicontroller.ziti" \
        --values https://docs.openziti.io/helm-charts/charts/ziti-controller/values-ingress-nginx.yaml >/dev/null
fi

for DEPLOYMENT in minicontroller-cert-manager trust-manager minicontroller; do
    echo "INFO: waiting for $DEPLOYMENT to be ready"
    kubectl wait deployments "$DEPLOYMENT" \
        --namespace ziti-controller \
        --for condition=Available=True \
        --timeout=240s >/dev/null
done

# xargs trims whitespace because minikube ssh returns a stray trailing '\r' after remote command output
MINIKUBE_NODE_INTERNAL=$(minikube --profile miniziti ssh 'grep host.minikube.internal /etc/hosts')

if [[ -n "${MINIKUBE_NODE_INTERNAL:-}" ]]; then
    # strip surrounding whitespace
    MINIKUBE_NODE_INTERNAL=$(xargs <<< "${MINIKUBE_NODE_INTERNAL}")
    echo "INFO: the minikube internal host record is \"${MINIKUBE_NODE_INTERNAL}\""
else
    echo "ERROR: failed to find minikube internal IP" >&2
    exit 1
fi

kubectl patch configmap "coredns" \
    --namespace kube-system \
    --patch="
data:
    Corefile: |
        .:53 {
            log
            errors
            health {
                lameduck 5s
            }
            ready
            kubernetes cluster.local in-addr.arpa ip6.arpa {
                pods insecure
                fallthrough in-addr.arpa ip6.arpa
                ttl 30
            }
            prometheus :9153
            hosts {
                ${MINIKUBE_NODE_INTERNAL}
                fallthrough
            }
            forward . /etc/resolv.conf {
                max_concurrent 1000
            }
            cache 30
            loop
            reload
            loadbalance
        }
        ziti:53 {
            errors
            cache 30
            forward . ${MINIKUBE_NODE_EXTERNAL}
        }
" >/dev/null

kubectl get pods \
    --namespace kube-system \
    | awk '/^coredns-/ {print $1}' \
    | xargs kubectl delete pods \
        --namespace kube-system >/dev/null

echo "INFO: waiting for coredns to be ready"

kubectl wait deployments "coredns" \
    --namespace kube-system \
    --for condition=Available=True >/dev/null

if kubectl run "dnstest" --rm --tty --stdin --image=busybox --restart=Never -- \
    nslookup minicontroller.ziti | grep "${MINIKUBE_NODE_EXTERNAL}" >/dev/null; then
    echo "INFO: cluster DNS is working"
else
    echo "ERROR: cluster DNS test failed" >&2
    exit 1
fi

kubectl get secrets "minicontroller-admin-secret" \
    --namespace ziti-controller \
    --output go-template='{{index .data "admin-password" | base64decode }}' \
    | xargs ziti edge login minicontroller.ziti:443 \
        --yes --username "admin" \
        --password >/dev/null

if ziti edge list edge-routers 'name="minirouter"' | grep -q minirouter; then
    ziti edge update edge-router "minirouter" \
        --role-attributes "public-routers" >/dev/null
else
    ziti edge create edge-router "minirouter" \
        --role-attributes "public-routers" \
        --tunneler-enabled \
        --jwt-output-file /tmp/minirouter.jwt >/dev/null
fi

if helm list --all --namespace ziti-router | grep -q minirouter; then
    helm upgrade "minirouter" openziti/ziti-router \
        --namespace ziti-router \
        --set enrollmentJwt=\ \
        --set edge.advertisedHost=minirouter.ziti \
        --set ctrl.endpoint=minicontroller-ctrl.ziti-controller.svc:6262 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-router/values-ingress-nginx.yaml >/dev/null
else
    helm install "minirouter" openziti/ziti-router \
        --namespace ziti-router --create-namespace \
        --set-file enrollmentJwt=/tmp/minirouter.jwt \
        --set edge.advertisedHost=minirouter.ziti \
        --set ctrl.endpoint=minicontroller-ctrl.ziti-controller.svc:6262 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-router/values-ingress-nginx.yaml >/dev/null
fi

echo "INFO: waiting for minirouter to be ready"

kubectl wait deployments "minirouter" \
    --namespace ziti-router \
    --for condition=Available=True >/dev/null

if ziti edge list edge-routers 'name="minirouter"' \
    | awk '/minirouter/ {print $6}' \
    | grep -q true; then
    echo "INFO: minirouter is online"
else
    echo "ERROR: minirouter is offline" >&2
    exit 1
fi

if helm --namespace ziti-console list --all | grep -q miniconsole; then
    helm upgrade "miniconsole" openziti/ziti-console \
        --namespace ziti-console \
        --set ingress.advertisedHost=miniconsole.ziti \
        --set settings.edgeControllers[0].url=https://minicontroller-client.ziti-controller.svc:443 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-console/values-ingress-nginx.yaml >/dev/null
else
    helm install "miniconsole" openziti/ziti-console \
        --namespace ziti-console --create-namespace \
        --set ingress.advertisedHost=miniconsole.ziti \
        --set settings.edgeControllers[0].url=https://minicontroller-client.ziti-controller.svc:443 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-console/values-ingress-nginx.yaml >/dev/null
fi

echo "INFO: waiting for miniconsole to be ready"

kubectl wait deployments "miniconsole" \
    --namespace ziti-console \
    --for condition=Available=True \
    --timeout=240s >/dev/null

if ! ziti edge list identities 'name="edge-client"' --csv | grep -q "edge-client"; then
    ziti edge create identity device "edge-client" \
        --jwt-output-file /tmp/edge-client.jwt --role-attributes testapi-clients >/dev/null
fi

if ! ziti edge list identities 'name="testapi-host"' --csv | grep -q "testapi-host"; then
    ziti edge create identity device "testapi-host" \
        --jwt-output-file /tmp/testapi-host.jwt --role-attributes testapi-hosts >/dev/null
fi
    
if ! ziti edge list configs 'name="testapi-intercept-config"' --csv | grep -q "testapi-intercept-config"; then
    ziti edge create config "testapi-intercept-config" intercept.v1 \
        '{"protocols":["tcp"],"addresses":["testapi.ziti"], "portRanges":[{"low":80, "high":80}]}' >/dev/null
fi
    
if ! ziti edge list configs 'name="testapi-host-config"' --csv | grep -q "testapi-host-config"; then
    ziti edge create config "testapi-host-config" host.v1 \
        '{"protocol":"tcp", "address":"httpbin","port":8080}' >/dev/null
fi
    
if ! ziti edge list services 'name="testapi-service"' --csv | grep -q "testapi-service"; then
    ziti edge create service "testapi-service" --configs testapi-intercept-config,testapi-host-config >/dev/null
fi
    
if ! ziti edge list service-policies 'name="testapi-bind-policy"' --csv | grep -q "testapi-bind-policy"; then
    ziti edge create service-policy "testapi-bind-policy" Bind \
        --service-roles '@testapi-service' --identity-roles '#testapi-hosts' >/dev/null
fi
    
if ! ziti edge list service-policies 'name="testapi-dial-policy"' --csv | grep -q "testapi-dial-policy"; then
    ziti edge create service-policy "testapi-dial-policy" Dial \
        --service-roles '@testapi-service' --identity-roles '#testapi-clients' >/dev/null
fi
    
if ! ziti edge list edge-router-policies 'name="public-routers"' --csv | grep -q "public-routers"; then
    ziti edge create edge-router-policy "public-routers" \
        --edge-router-roles '#public-routers' --identity-roles '#all' >/dev/null
fi
    
if ! ziti edge list service-edge-router-policies 'name="public-routers"' --csv | grep -q "public-routers"; then
    ziti edge create service-edge-router-policy "public-routers" \
        --edge-router-roles '#public-routers' --service-roles '#all' >/dev/null
fi

if [[ -s /tmp/testapi-host.json ]]; then
    echo "WARN: /tmp/testapi-host.json exists, not enrolling 'testapi-host'."\
        "If the file was left over from a prior run"\
        " then delete it and re-run this script." >&2
else
    ziti edge enroll /tmp/testapi-host.jwt >/dev/null
fi

if helm list --all --namespace default | grep -q testapi-host; then
    helm upgrade "testapi-host" openziti/httpbin \
        --set-file zitiIdentity=/tmp/testapi-host.json \
        --set zitiServiceName=testapi-service >/dev/null
else
    helm install "testapi-host" openziti/httpbin \
        --set-file zitiIdentity=/tmp/testapi-host.json \
        --set zitiServiceName=testapi-service >/dev/null
fi

kubectl get secrets "minicontroller-admin-secret" \
    --namespace ziti-controller \
    --output go-template='{{"\nINFO: Your OpenZiti Console http://miniconsole.ziti password for \"admin\" is: "}}{{index .data "admin-password" | base64decode }}{{"\n\n"}}'

echo "INFO: Success! Remember to add your edge client identity '/tmp/edge-client.jwt' in your client tunneler, e.g. Ziti Desktop Edge."
