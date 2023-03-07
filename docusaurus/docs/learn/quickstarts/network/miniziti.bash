#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

function banner(){
    cat <<BANNER
                  __   ___   
    |\/| | |\ | |  / |  |  | 
    |  | | | \| | /_ |  |  | 

BANNER

    if (( $# )); then
        echo -e "      .: $* edition :.\n"
    fi
}

function _usage(){
    banner
    echo -e "\n"\
            "\t --quiet\tsuppress INFO messages\n"\
            "\t --verbose\tshow DEBUG messages\n"\
            "\t --delete\tdelete miniziti\n"\
            "\t --profile\tMINIKUBE_PROFILE (default is \"miniziti\")"
}

function checkDns(){
    # only param is an IPv4
    [[ $# -eq 1 && $1 =~ ([0-9]{1,3}\.){3}[0-9]{1,3} ]] || {
        echo "ERROR: need IPv4 of miniziti ingress as only param to checkDns()" >&2
        return 1
    }
    echo "DEBUG: checking dns to ensure minicontroller.ziti resolves to $1" >&3
    if grep -qE "^${1//./\\.}.*minicontroller.ziti" /etc/hosts \
        || nslookup minicontroller.ziti | grep -q "${1//./\\.}"; then
        echo "DEBUG: host dns found expected minikube ingress IP '$1'" >&3
        return 0
    else
        echo "ERROR: minicontroller.ziti does not resolve to '$1'. Did you add the record in /etc/hosts?"
        return 1
    fi
}

function deleteMiniziti(){ 
    if (( $# )) && [[ $1 =~ ^[0-9]+$ ]]; then
        WAIT="$1"
        shift
    else
        WAIT=10
    fi
    (( $# )) && {
        echo "WARN: ignorring extra params '$*'" >&2
    }
    echo "WARN: deleting ${MINIKUBE_PROFILE} in ${WAIT}s" >&2
    sleep "$WAIT"
    echo "INFO: waiting for ${MINIKUBE_PROFILE} to be deleted"
    minikube --profile "${MINIKUBE_PROFILE}" delete >/dev/null
}

function main(){
    # require commands
    declare -a BINS=(jq ziti minikube kubectl helm)
    for BIN in "${BINS[@]}"; do
        if ! command -v "$BIN" &>/dev/null; then
            echo "ERROR: this script requires commands '${BINS[*]}'. Please install on the search PATH and try again." >&2
            $BIN || exit 1
        fi
    done

    # open a descriptor for debug messages
    exec 3>/dev/null

    while (( $# )); do
        case "$1" in
            -p|--profile)   MINIKUBE_PROFILE="$2"
                            shift 2
            ;;
            -d|--delete)    DELETE_MINIZITI=1
                            shift
            ;;
            -q|--quiet)     exec > /dev/null
                            shift
            ;;
            -v|--verbose|--debug)
                            exec 3>&1
                            shift
            ;;
            *)              _usage
                            exit
            ;;
        esac
    done

    if [[ ${MINIKUBE_PROFILE:="miniziti"} == "miniziti" ]]; then
        banner
    else
        # sanity check the profile name input
        if [[ ${MINIKUBE_PROFILE} =~ ^- ]]; then
            # in case the arg value is another option instead of the profile name
            echo "ERROR: --profile needs a profile name not starting with a hyphen" >&2
            _usage; exit 1
        fi
        # print the alternative profile name if not default "miniziti"
        banner "$MINIKUBE_PROFILE"
    fi

    (( ${DELETE_MINIZITI:-0} )) && {
        deleteMiniziti 10
        exit 0
    }

    # start unless running
    echo "INFO: waiting for minikube to be ready"
    if ! minikube --profile "${MINIKUBE_PROFILE}" status 2>/dev/null | grep -q "apiserver: Running"; then
        minikube --profile "${MINIKUBE_PROFILE}" start >/dev/null
    fi

    MINIKUBE_NODE_EXTERNAL=$(minikube --profile "${MINIKUBE_PROFILE}" ip)

    if [[ -n "${MINIKUBE_NODE_EXTERNAL:-}" ]]; then
        echo "DEBUG: the minikube external IP is ${MINIKUBE_NODE_EXTERNAL}" >&3
    else
        echo "ERROR: failed to find minikube external IP" >&2
        exit 1
    fi

    # verify current context can connect to apiserver
    kubectl cluster-info >/dev/null
    echo "DEBUG: kubectl successfully obtained cluster-info from apiserver" >&3

    # enable ssl-passthrough for OpenZiti ingresses
    if kubectl get deployment "ingress-nginx-controller" \
        --namespace ingress-nginx \
        --output 'go-template={{ (index .spec.template.spec.containers 0).args }}' 2>/dev/null \
        | grep -q enable-ssl-passthrough; then
        echo "DEBUG: ingress-nginx has ssl-passthrough enabled" >&3
    else
        echo "DEBUG: installing ingress-nginx" >&3
        # enable minikube addons for ingress-nginx
        minikube --profile "${MINIKUBE_PROFILE}" addons enable ingress >/dev/null
        minikube --profile "${MINIKUBE_PROFILE}" addons enable ingress-dns >/dev/null

        echo "DEBUG: patching ingress-nginx deployment to enable ssl-passthrough" >&3
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
        echo "DEBUG: detected WSL, probing for running minikube tunnel" >&3
        if ! pgrep -f "minikube --profile ${MINIKUBE_PROFILE} tunnel" >/dev/null; then
            echo -e "INFO: detected WSL. minikube tunnel required."\
                    " In another terminal, run the following command. Then re-run this script."\
                    "\n\n\tminikube --profile ${MINIKUBE_PROFILE} tunnel"
            exit 1
        fi
        # recommend /etc/hosts change unless dns is configured to reach the minikube node IP
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

    echo "DEBUG: applying Custom Resource Definitions: Certificate, Issuer, and Bundle" >&3
    kubectl apply \
        --filename https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.crds.yaml >/dev/null
    kubectl apply \
        --filename https://raw.githubusercontent.com/cert-manager/trust-manager/v0.4.0/deploy/crds/trust.cert-manager.io_bundles.yaml >/dev/null

    if helm repo list | grep -q openziti; then
        echo "DEBUG: refreshing OpenZiti Helm Charts" >&3
        helm repo update openziti >/dev/null
    else
        echo "INFO: subscribing to OpenZiti Helm Charts"
        helm repo add openziti https://docs.openziti.io/helm-charts/ >/dev/null
    fi

    if helm list --namespace ziti-controller --all | grep -q minicontroller; then
        echo "INFO: upgrading openziti controller"
        helm upgrade "minicontroller" openziti/ziti-controller \
            --namespace ziti-controller \
            --set clientApi.advertisedHost="minicontroller.ziti" \
            --values https://docs.openziti.io/helm-charts/charts/ziti-controller/values-ingress-nginx.yaml >/dev/null
    else
        echo "INFO: installing openziti controller"
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
    echo "DEBUG: probing minikube node for internal host record" >&3
    MINIKUBE_NODE_INTERNAL=$(minikube --profile "${MINIKUBE_PROFILE}" ssh 'grep host.minikube.internal /etc/hosts')

    if [[ -n "${MINIKUBE_NODE_INTERNAL:-}" ]]; then
        # strip surrounding whitespace
        MINIKUBE_NODE_INTERNAL=$(xargs <<< "${MINIKUBE_NODE_INTERNAL}")
        echo "DEBUG: the minikube internal host record is \"${MINIKUBE_NODE_INTERNAL}\"" >&3
    else
        echo "ERROR: failed to find minikube internal IP" >&2
        exit 1
    fi

    echo "DEBUG: patching coredns configmap with *.ziti forwarder to minikube ingress-dns nameserver" >&3
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

    echo "DEBUG: deleting coredns pod so a new one will have modified Corefile" >&3
    kubectl get pods \
        --namespace kube-system \
        | awk '/^coredns-/ {print $1}' \
        | xargs kubectl delete pods \
            --namespace kube-system >/dev/null

    echo "DEBUG: waiting for cluster dns to be ready" >&3
    kubectl wait deployments "coredns" \
        --namespace kube-system \
        --for condition=Available=True >/dev/null

    if kubectl run "dnstest" --rm --tty --stdin --image=busybox --restart=Never -- \
        nslookup minicontroller.ziti | grep "${MINIKUBE_NODE_EXTERNAL}" >/dev/null; then
        echo "INFO: cluster dns is working"
    else
        echo "ERROR: cluster dns test failed" >&2
        exit 1
    fi

    echo "DEBUG: fetching admin password from k8s secret to log in to ziti mgmt" >&3
    kubectl get secrets "minicontroller-admin-secret" \
        --namespace ziti-controller \
        --output go-template='{{index .data "admin-password" | base64decode }}' \
        | xargs ziti edge login minicontroller.ziti:443 \
            --yes --username "admin" \
            --password >/dev/null

    if ziti edge list edge-routers 'name="minirouter"' | grep -q minirouter; then
        echo "DEBUG: updating minirouter" >&3
        ziti edge update edge-router "minirouter" \
            --role-attributes "public-routers" >/dev/null
    else
        echo "DEBUG: creating minirouter" >&3
        ziti edge create edge-router "minirouter" \
            --role-attributes "public-routers" \
            --tunneler-enabled \
            --jwt-output-file /tmp/minirouter.jwt >/dev/null
    fi

    if helm list --all --namespace ziti-router | grep -q minirouter; then
        echo "DEBUG: upgrading router chart as 'minirouter'" >&3
        helm upgrade "minirouter" openziti/ziti-router \
            --namespace ziti-router \
            --set enrollmentJwt=\ \
            --set edge.advertisedHost=minirouter.ziti \
            --set ctrl.endpoint=minicontroller-ctrl.ziti-controller.svc:6262 \
            --values https://docs.openziti.io/helm-charts/charts/ziti-router/values-ingress-nginx.yaml >/dev/null
    else
        echo "DEBUG: installing router chart as 'minirouter'" >&3
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

    echo "DEBUG: probing minirouter for online status" >&3
    if ziti edge list edge-routers 'name="minirouter"' \
        | awk '/minirouter/ {print $6}' \
        | grep -q true; then
        echo "INFO: minirouter is online"
    else
        echo "ERROR: minirouter is offline" >&2
        exit 1
    fi

    if helm --namespace ziti-console list --all | grep -q miniconsole; then
        echo "DEBUG: upgrading console chart as 'miniconsole'" >&3
        helm upgrade "miniconsole" openziti/ziti-console \
            --namespace ziti-console \
            --set ingress.advertisedHost=miniconsole.ziti \
            --set settings.edgeControllers[0].url=https://minicontroller-client.ziti-controller.svc:443 \
            --values https://docs.openziti.io/helm-charts/charts/ziti-console/values-ingress-nginx.yaml >/dev/null
    else
        echo "DEBUG: installing console chart as 'miniconsole'" >&3
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
        echo "DEBUG: creating identity edge-client" >&3
        ziti edge create identity device "edge-client" \
            --jwt-output-file /tmp/miniziti-client.jwt --role-attributes testapi-clients >/dev/null
    else
        echo "DEBUG: ignoring identity edge-client" >&3
    fi

    if ! ziti edge list identities 'name="testapi-host"' --csv | grep -q "testapi-host"; then
        echo "DEBUG: creating identity testapi-host" >&3
        ziti edge create identity device "testapi-host" \
            --jwt-output-file /tmp/testapi-host.jwt --role-attributes testapi-hosts >/dev/null
    else
        echo "DEBUG: ignoring identity testapi-host" >&3
    fi
        
    if ! ziti edge list configs 'name="testapi-intercept-config"' --csv | grep -q "testapi-intercept-config"; then
        echo "DEBUG: creating config testapi-intercept-config" >&3
        ziti edge create config "testapi-intercept-config" intercept.v1 \
            '{"protocols":["tcp"],"addresses":["testapi.ziti"], "portRanges":[{"low":80, "high":80}]}' >/dev/null
    else
        echo "DEBUG: ignoring config testapi-intercept-config" >&3
    fi
        
    if ! ziti edge list configs 'name="testapi-host-config"' --csv | grep -q "testapi-host-config"; then
        echo "DEBUG: creating config testapi-host-config" >&3
        ziti edge create config "testapi-host-config" host.v1 \
            '{"protocol":"tcp", "address":"httpbin","port":8080}' >/dev/null
    else
        echo "DEBUG: ignoring config testapi-host-config" >&3
    fi
        
    if ! ziti edge list services 'name="testapi-service"' --csv | grep -q "testapi-service"; then
        echo "DEBUG: creating service testapi-service" >&3
        ziti edge create service "testapi-service" --configs testapi-intercept-config,testapi-host-config >/dev/null
    else
        echo "DEBUG: ignoring service testapi-service" >&3
    fi
        
    if ! ziti edge list service-policies 'name="testapi-bind-policy"' --csv | grep -q "testapi-bind-policy"; then
        echo "DEBUG: creating service-policy testapi-bind-policy" >&3
        ziti edge create service-policy "testapi-bind-policy" Bind \
            --service-roles '@testapi-service' --identity-roles '#testapi-hosts' >/dev/null
    else
        echo "DEBUG: ignoring service-policy testapi-bind-policy" >&3
    fi
        
    if ! ziti edge list service-policies 'name="testapi-dial-policy"' --csv | grep -q "testapi-dial-policy"; then
        echo "DEBUG: creating service-policy testapi-dial-policy" >&3
        ziti edge create service-policy "testapi-dial-policy" Dial \
            --service-roles '@testapi-service' --identity-roles '#testapi-clients' >/dev/null
    else
        echo "DEBUG: ignoring service-policy testapi-dial-policy" >&3
    fi
        
    if ! ziti edge list edge-router-policies 'name="public-routers"' --csv | grep -q "public-routers"; then
        echo "DEBUG: creating edge-router-policy public-routers" >&3
        ziti edge create edge-router-policy "public-routers" \
            --edge-router-roles '#public-routers' --identity-roles '#all' >/dev/null
    else
        echo "DEBUG: ignoring edge-router-policy public-routers" >&3
    fi
        
    if ! ziti edge list service-edge-router-policies 'name="public-routers"' --csv | grep -q "public-routers"; then
        echo "DEBUG: creating service-edge-router-policy public-routers" >&3
        ziti edge create service-edge-router-policy "public-routers" \
            --edge-router-roles '#public-routers' --service-roles '#all' >/dev/null
    else
        echo "DEBUG: ignoring service-edge-router-policy public-routers" >&3
    fi

    if [[ -s /tmp/testapi-host.json ]]; then
        echo "WARN: /tmp/testapi-host.json exists, not enrolling 'testapi-host'."\
            "If the file was left over from a prior run"\
            " then delete it and re-run this script." >&2
    else
        echo "DEBUG: enrolling /tmp/testapi-host.jwt" >&3
        ziti edge enroll /tmp/testapi-host.jwt >/dev/null
    fi

    if helm list --all --namespace default | grep -q testapi-host; then
        echo "DEBUG: upgrading httpbin chart as 'testapi-host'" >&3
        helm upgrade "testapi-host" openziti/httpbin \
            --set-file zitiIdentity=/tmp/testapi-host.json \
            --set zitiServiceName=testapi-service >/dev/null
    else
        echo "DEBUG: installing httpbin chart as 'testapi-host'" >&3
        helm install "testapi-host" openziti/httpbin \
            --set-file zitiIdentity=/tmp/testapi-host.json \
            --set zitiServiceName=testapi-service >/dev/null
    fi

    kubectl get secrets "minicontroller-admin-secret" \
        --namespace ziti-controller \
        --output go-template='{{"\nINFO: Your OpenZiti Console is here:\thttp://miniconsole.ziti\nINFO: The password for \"admin\" is:\t"}}{{index .data "admin-password" | base64decode }}{{"\n\n"}}'

    echo "INFO: Success! Remember to add your edge client identity '/tmp/miniziti-client.jwt' in your client tunneler, e.g. Ziti Desktop Edge."
}

main "$@"
