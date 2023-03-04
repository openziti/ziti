#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# require commands
declare -a BINS=(jq ziti minikube kubectl helm)
for BIN in ${BINS[*]}; do
    if ! command -v $BIN &>/dev/null; then
        echo "ERROR: this script requires '$BIN'. Please install it on the search PATH and try again." >&2
        $BIN || exit 1
    fi
done

# start unless running
if ! minikube --profile miniziti status | grep -q "apiserver: Running"; then
    minikube --profile miniziti start
fi

# verify current context can connect to apiserver
kubectl cluster-info

# enable minikube addons for ingress-nginx
minikube --profile miniziti addons enable ingress
minikube --profile miniziti addons enable ingress-dns

# enable ssl-passthrough for OpenZiti ingresses
kubectl patch deployment \
    --namespace ingress-nginx "ingress-nginx-controller" \
    --type='json' \
    --patch='[{"op": "add",
        "path": "/spec/template/spec/containers/0/args/-",
        "value":"--enable-ssl-passthrough"
       }]'

# wait for ingress-nginx
kubectl wait jobs "ingress-nginx-admission-patch" \
    --namespace ingress-nginx \
        --for condition=complete \
        --timeout=120s

kubectl wait pods \
    --namespace ingress-nginx \
    --for=condition=ready \
    --selector=app.kubernetes.io/component=controller \
    --timeout=120s

if grep -qi "microsoft" /proc/sys/kernel/osrelease; then
    echo "INFO: detected WSL. Privileged minikube tunnel required."\
         "You may be prompted for your WSL user's password to proceed."\
         "minikube tunnel will write log messages in /tmp/minitunnel.log"
    # this approach allows us to grab stdin and prompt for password before proceeding in the background
    sudo --background bash -c "
        MINIKUBE_HOME=${HOME}/.minikube \
        KUBECONFIG=${HOME}/.kube/config \
            minikube --profile miniziti tunnel &> /tmp/minitunnel.log
    "
fi

kubectl apply \
   --filename https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.crds.yaml
kubectl apply \
   --filename https://raw.githubusercontent.com/cert-manager/trust-manager/v0.4.0/deploy/crds/trust.cert-manager.io_bundles.yaml

if helm list --namespace ziti-controller --all | grep -q minicontroller; then
    helm upgrade "minicontroller" openziti/ziti-controller \
        --namespace ziti-controller \
        --set clientApi.advertisedHost="minicontroller.ziti" \
        --values https://docs.openziti.io/helm-charts/charts/ziti-controller/values-ingress-nginx.yaml
else
    helm install "minicontroller" openziti/ziti-controller \
        --namespace ziti-controller --create-namespace \
        --set clientApi.advertisedHost="minicontroller.ziti" \
        --values https://docs.openziti.io/helm-charts/charts/ziti-controller/values-ingress-nginx.yaml
fi

kubectl wait deployments "minicontroller" \
    --namespace ziti-controller \
    --for condition=Available=True \
    --timeout=240s

MINIKUBE_EXTERNAL_IP=$(minikube --profile miniziti ip)

if [[ -n "${MINIKUBE_EXTERNAL_IP:-}" ]]; then
    echo "INFO: the minikube external IP is ${MINIKUBE_EXTERNAL_IP}"
else
    echo "ERROR: failed to find minikube external IP" >&2
    exit 1
fi

# xargs trims whitespace because minikube ssh returns a stray trailing '\r' after remote command output
MINIKUBE_INTERNAL_HOST=$(minikube --profile miniziti ssh 'grep host.minikube.internal /etc/hosts')

if [[ -n "${MINIKUBE_INTERNAL_HOST:-}" ]]; then
    echo "INFO: the minikube internal host record is \"${MINIKUBE_INTERNAL_HOST}\""
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
           ${MINIKUBE_INTERNAL_HOST}
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
       forward . ${MINIKUBE_EXTERNAL_IP}
    }
"

kubectl get pods \
    --namespace kube-system \
    | awk '/^coredns-/ {print $1}' \
    | xargs -rl kubectl delete pods \
        --namespace kube-system

kubectl wait deployments "coredns" \
    --namespace kube-system \
    --for condition=Available=True

if kubectl run "dnstest" --rm --tty --stdin --image=busybox --restart=Never -- \
    nslookup minicontroller.ziti | grep "${MINIKUBE_EXTERNAL_IP}" >/dev/null; then
    echo "INFO: cluster DNS is working"
else
    echo "ERROR: cluster DNS test failed" >&2
    exit 1
fi

kubectl get secrets "minicontroller-admin-secret" \
            --namespace ziti-controller \
            --output go-template='{{index .data "admin-password" | base64decode }}' \
    | xargs -rl ziti edge login minicontroller.ziti:443 \
        --yes --username "admin" \
        --password

if ziti edge list edge-routers 'name="minirouter"' | grep -q minirouter; then
    ziti edge update edge-router "minirouter" \
        --role-attributes "public-routers"
else
    ziti edge create edge-router "minirouter" \
        --role-attributes "public-routers" \
        --tunneler-enabled \
        --jwt-output-file /tmp/minirouter.jwt
fi

if helm list --all --namespace ziti-router | grep -q minirouter; then
    helm upgrade "minirouter" openziti/ziti-router \
        --namespace ziti-router \
        --set enrollmentJwt=\ \
        --set edge.advertisedHost=minirouter.ziti \
        --set ctrl.endpoint=minicontroller-ctrl.ziti-controller.svc:6262 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-router/values-ingress-nginx.yaml
else
    helm install "minirouter" openziti/ziti-router \
        --namespace ziti-router --create-namespace \
        --set-file enrollmentJwt=/tmp/minirouter.jwt \
        --set edge.advertisedHost=minirouter.ziti \
        --set ctrl.endpoint=minicontroller-ctrl.ziti-controller.svc:6262 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-router/values-ingress-nginx.yaml
fi

kubectl wait deployments "minirouter" \
    --namespace ziti-router \
    --for condition=Available=True

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
        --values https://docs.openziti.io/helm-charts/charts/ziti-console/values-ingress-nginx.yaml
else
    helm install "miniconsole" openziti/ziti-console \
        --namespace ziti-console --create-namespace \
        --set ingress.advertisedHost=miniconsole.ziti \
        --set settings.edgeControllers[0].url=https://minicontroller-client.ziti-controller.svc:443 \
        --values https://docs.openziti.io/helm-charts/charts/ziti-console/values-ingress-nginx.yaml
fi

kubectl wait deployments "miniconsole" \
    --namespace ziti-console \
    --for condition=Available=True \
    --timeout=240s

if ! ziti edge list identities 'name="edge-client"' | grep -q "edge-client"; then
    ziti edge create identity device "edge-client" \
        --jwt-output-file /tmp/edge-client.jwt --role-attributes testapi-clients
fi

if ! ziti edge list identities 'name="testapi-host"' | grep -q "testapi-host"; then
    ziti edge create identity device "testapi-host" \
        --jwt-output-file /tmp/testapi-host.jwt --role-attributes testapi-hosts
fi
    
if ! ziti edge list configs 'name="testapi-intercept-config"' | grep -q "testapi-intercept-config"; then
    ziti edge create config "testapi-intercept-config" intercept.v1 \
        '{"protocols":["tcp"],"addresses":["testapi.ziti"], "portRanges":[{"low":80, "high":80}]}'
fi
    
if ! ziti edge list configs 'name="testapi-host-config"' | grep -q "testapi-host-config"; then
    ziti edge create config "testapi-host-config" host.v1 \
        '{"protocol":"tcp", "address":"httpbin","port":8080}'
fi
    
if ! ziti edge list services 'name="testapi-service"' | grep -q "testapi-service"; then
    ziti edge create service "testapi-service" --configs testapi-intercept-config,testapi-host-config
fi
    
if ! ziti edge list service-policies 'name="testapi-bind-policy"' | grep -q "testapi-bind-policy"; then
    ziti edge create service-policy "testapi-bind-policy" Bind \
        --service-roles '@testapi-service' --identity-roles '#testapi-hosts'
fi
    
if ! ziti edge list service-policies 'name="testapi-dial-policy"' | grep -q "testapi-dial-policy"; then
    ziti edge create service-policy "testapi-dial-policy" Dial \
        --service-roles '@testapi-service' --identity-roles '#testapi-clients'
fi
    
if ! ziti edge list edge-router-policies 'name="public-routers"' | grep -q "public-routers"; then
    ziti edge create edge-router-policy "public-routers" \
        --edge-router-roles '#public-routers' --identity-roles '#all'
fi
    
if ! ziti edge list service-edge-router-policies 'name="public-routers"' | grep -q "public-routers"; then
    ziti edge create service-edge-router-policy "public-routers" \
        --edge-router-roles '#public-routers' --service-roles '#all'
fi

if [[ -s /tmp/testapi-host.json ]]; then
    echo "WARN: /tmp/testapi-host.json exists, not enrolling 'testapi-host'."\
         "If the file was left over from a prior exercise and is no longer"\
         "working then delete it and re-run this script." >&2
else
    ziti edge enroll /tmp/testapi-host.jwt
fi

if helm list --all --namespace default | grep -q testapi-host; then
    helm upgrade "testapi-host" openziti/httpbin \
       --set-file zitiIdentity=/tmp/testapi-host.json \
       --set zitiServiceName=testapi-service
else
    helm install "testapi-host" openziti/httpbin \
       --set-file zitiIdentity=/tmp/testapi-host.json \
       --set zitiServiceName=testapi-service
fi

kubectl get secrets "minicontroller-admin-secret" \
    --namespace ziti-controller \
    --output go-template='{{"\nINFO: Your OpenZiti Console http://miniconsole.ziti password for \"admin\" is: "}}{{index .data "admin-password" | base64decode }}{{"\n\n"}}'

