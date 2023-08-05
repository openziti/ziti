#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

banner(){
    cat <<BANNER
                  __   ___
    |\/| | |\ | |  / |  |  |
    |  | | | \| | /_ |  |  |

BANNER

    if (( $# )); then
        echo -e "   .: $* :.\n"
    fi
}

_usage(){
    if (( $# )); then
        echo -e "\nERROR: unexpected arg '$1'" >&2
    else
        banner
    fi
    echo -e "\n COMMANDS\n"\
            "   start\t\tstart miniziti (default)\n"\
            "   delete\t\tdelete miniziti\n"\
            "   help\t\tshow these usage hints\n"\
            "\n OPTIONS\n"\
            "   --quiet\t\tsuppress INFO messages\n"\
            "   --verbose\t\tshow DEBUG messages\n"\
            "   --profile\t\tMINIKUBE_PROFILE (miniziti)\n"\
            "   --namespace\t\tZITI_NAMESPACE (MINIKUBE_PROFILE)\n"\
            "   --no-hosts\t\tdon't use local hosts DB or ingress-dns nameserver\n"\
            "\n DEBUG\n"\
            "   --charts\t\tZITI_CHARTS (openziti) alternative charts repo\n"\
            "   --now\t\teliminate safety waits, e.g., before deleting miniziti\n"\
            "   --\t\t\tMINIKUBE_START_ARGS args after -- passed to minikube start\n"
}

checkDns(){
    # only param is an IPv4
    [[ $# -eq 1 && $1 =~ ([0-9]{1,3}\.){3}[0-9]{1,3} ]] || {
        logError "need IPv4 of miniziti ingress as only param to checkDns()"
        return 1
    }
    logDebug "checking dns to ensure miniziti-controller.${MINIZITI_INGRESS_ZONE} resolves to $1"
    if grep -qE "^${1//./\\.}.*miniziti-controller.${MINIZITI_INGRESS_ZONE}" /etc/hosts \
        || nslookup "miniziti-controller.${MINIZITI_INGRESS_ZONE}" | grep -q "${1//./\\.}"; then
        logDebug "host dns found expected minikube ingress IP '$1'"
        return 0
    else
        logError "miniziti-controller.${MINIZITI_INGRESS_ZONE} does not resolve to '$1'. Did you add the record in /etc/hosts?"
        return 1
    fi
}

deleteMiniziti(){
    local WAIT=10
    if [[ $1 =~ ^[0-9]+$ ]]; then
        WAIT="$1"
        shift
    else
        logDebug "no integer param detected to deleteMiniziti(), using default wait time ${WAIT}s"
    fi
    (( SAFETY_WAIT )) && {
        logWarn "deleting ${MINIKUBE_PROFILE} in ${WAIT}s" >&2
        sleep "$WAIT"
    }
    logInfo "waiting for ${MINIKUBE_PROFILE} to be deleted"
    minikube --profile "${MINIKUBE_PROFILE}" delete >&3
}

detectOs(){
    if grep -qi "microsoft" /proc/sys/kernel/osrelease 2>/dev/null; then
        logDebug "detected Windows OS"
        echo "Windows"
    elif [[ ${OSTYPE:-} =~ [Dd]arwin ]]; then
        logDebug "detected macOS OS"
        echo "macOS"
    elif [[ ${OSTYPE:-} =~ [Ll]inux ]]; then
        logDebug "detected Linux OS"
        echo "Linux"
    else
        logError "failed to detect OS"
        return 1
    fi
}

getClientOttPath(){
    local ID_PATH="/tmp/${MINIKUBE_PROFILE}-client.jwt"
    if [[ $# -eq 1 ]]; then
        case $1 in
            macOS|Linux)     echo "$ID_PATH"
            ;;
            Windows)         wslpath -w "$ID_PATH"
            ;;
        esac
    else
        logError "getClientOttPath() takes one param, DETECTED_OS"
        return 1
    fi

}

testClusterDns(){
    if kubectl run "dnstest" --rm --tty --stdin --image busybox --restart Never -- \
        nslookup "miniziti-controller.${MINIZITI_INGRESS_ZONE}" | grep "$1" >&3; then
        logInfo "cluster dns test succeeded"
    else
        logError "cluster dns test failed"
        return 1
    fi
}

validateDnsName(){
    if ! [[ $# -eq 1 ]]; then
        logError "validateDnsName() takes one string param"
        return 1
    fi
    if grep -qP '(?=^.{4,253}$)(^(?:[a-zA-Z0-9](?:(?:[a-zA-Z0-9\-]){0,61}[a-zA-Z0-9])?)+[a-zA-Z0-9]$)' <<< "$1"; then
        logDebug "'$1' is a valid DNS name"
        return 0
    else
        logError "'$1' could not be validated as an unqualified DNS name which is limited to at least four alphanumeric and hyphen characters, starts with a letter, and does not end with a hyphen."
        return 1
    fi
}

logInfo() {
    (( $# )) || {
        logError "logInfo() takes 1 or more args"
        return 1
    }
    echo "INFO: $*"
}

logError() {
    (( $# )) || {
        echo "ERROR: logError() takes 1 or more args" >&2
        return 1
    }
    echo "ERROR: $*" >&2
}

logDebug() {
    (( $# )) || {
        logError "logDebug() takes 1 or more args"
        return 1
    }
    echo "DEBUG: $*" >&3
}

main(){
    # require commands
    declare -a BINS=(ziti minikube kubectl helm)
    for BIN in "${BINS[@]}"; do
        if ! command -v "$BIN" &>/dev/null; then
            logError "this script requires commands '${BINS[*]}'. Please install on the search PATH and try again."
            $BIN || exit 1
        fi
    done

    # open a descriptor for debug messages
    exec 3>/dev/null

    # local strings with defaults that never produce an error
    declare DETECTED_OS \
            MINIKUBE_PROFILE="miniziti" \
            ZITI_NAMESPACE \
            DELETE_MINIZITI=0 \
            SAFETY_WAIT=1 \
            MINIKUBE_NODE_EXTERNAL \
            ZITI_CHARTS="openziti" \
            MINIZITI_HOSTS=1 \
            MINIZITI_INGRESS_ZONE="ziti" \
            MINIZITI_INTERCEPT_ZONE="miniziti"
    # local arrays with defaults that never produce an error
    declare -a MINIKUBE_START_ARGS=()

    # local defaults that are inherited or may error
    DETECTED_OS="$(detectOs)"
    : "${DEBUG_MINIKUBE_TUNNEL:=0}"  # set env = 1 to trigger the minikube tunnel probe


    while (( $# )); do
        case "$1" in
            start)          shift
            ;;
            delete)         DELETE_MINIZITI=1
                            shift
            ;;
            -p|--profile)   validateDnsName "$2"
                            MINIKUBE_PROFILE="$2"
                            shift 2
            ;;
            -n|--namespace) ZITI_NAMESPACE="$2"
                            shift 2
            ;;
            --charts)       ZITI_CHARTS="$2"
                            shift 2
            ;;
            -q|--quiet)     exec > /dev/null
                            shift
            ;;
            -v|--verbose|--debug)
                            exec 3>&1
                            shift
            ;;
            --now)          SAFETY_WAIT=0
                            shift
            ;;
            --no-hosts)     MINIZITI_HOSTS=0
                            shift
            ;;
            --)             shift
                            mapfile -t -n1 MINIKUBE_START_ARGS <<< "$*"
                            shift $#
                            if (( ${#MINIKUBE_START_ARGS[*]} )) && [[ -n "${MINIKUBE_START_ARGS[0]}" ]]; then
                                logDebug "passing ${#MINIKUBE_START_ARGS[*]} args to minikube start: '${MINIKUBE_START_ARGS[*]}'"
                            else
                                MINIKUBE_START_ARGS=()
                            fi
            ;;
            -h|*help)       _usage
                            exit 0
            ;;
            *)              _usage "$1"
                            exit
            ;;
        esac
    done

    if [[ ${MINIKUBE_PROFILE} == "miniziti" ]]; then
        banner
    else
        # sanity check the profile name input
        if [[ ${MINIKUBE_PROFILE} =~ ^- ]]; then
            # in case the arg value is another option instead of the profile name
            logError "--profile needs a profile name not starting with a hyphen"
            _usage; exit 1
        fi
        # print the alternative profile name if not default "miniziti"
        banner "$MINIKUBE_PROFILE"
    fi

    # delete and exit if --delete
    (( DELETE_MINIZITI )) && {
        deleteMiniziti 10
        exit 0
    }

    : "${ZITI_NAMESPACE:=${MINIKUBE_PROFILE}}"

    #
    ## Ensure Minikube is Started and Configured
    #

    # run 'minikube start' if not running or any extra start args are present
    logInfo "waiting for minikube to be ready"
    if  ! minikube status \
            --profile "${MINIKUBE_PROFILE}" 2>/dev/null \
        | grep -q "apiserver: Running" \
        || (( ${#MINIKUBE_START_ARGS[*]} )); then
        logDebug "apiserver not running or got extra start args, running 'minikube start'"
        minikube start \
            --profile "${MINIKUBE_PROFILE}" \
            "${MINIKUBE_START_ARGS[@]}" >&3
    else
        logDebug "apiserver is running, not starting minikube"
    fi

    MINIKUBE_NODE_EXTERNAL=$(minikube --profile "${MINIKUBE_PROFILE}" ip)
    # if --no-hosts then build a new zone name for RFC-1918 wildcard DNS
    (( MINIZITI_HOSTS )) || {
        MINIZITI_INGRESS_ZONE="${MINIKUBE_NODE_EXTERNAL}.mini.openziti.io"
        logDebug "DNS wildcard zone for ingresses is ${MINIZITI_INGRESS_ZONE}"
    }
    if [[ -n "${MINIKUBE_NODE_EXTERNAL:-}" ]]; then
        logDebug "the minikube external IP is ${MINIKUBE_NODE_EXTERNAL}"
    else
        logError "failed to find minikube external IP"
        exit 1
    fi

    # verify current context can connect to apiserver
    kubectl cluster-info >&3
    logDebug "kubectl successfully obtained cluster-info from apiserver"

    # enable ssl-passthrough for OpenZiti ingresses
    if kubectl get deployment "ingress-nginx-controller" \
        --namespace ingress-nginx \
        --output 'go-template={{ (index .spec.template.spec.containers 0).args }}' 2>/dev/null \
        | grep -q enable-ssl-passthrough; then
        logDebug "ingress-nginx has ssl-passthrough enabled"
    else
        logDebug "installing ingress-nginx"
        # enable minikube addons for ingress-nginx
        minikube addons enable ingress \
            --profile "${MINIKUBE_PROFILE}" >&3
        # enable minikube addon ingress-dns unless --no-hosts
        (( MINIZITI_HOSTS )) && {
            minikube addons enable ingress-dns \
                --profile "${MINIKUBE_PROFILE}" >&3
        }
        logDebug "patching ingress-nginx deployment to enable ssl-passthrough"
        kubectl patch deployment "ingress-nginx-controller" \
            --namespace ingress-nginx \
            --type json \
            --patch '[{"op": "add",
                "path": "/spec/template/spec/containers/0/args/-",
                "value":"--enable-ssl-passthrough"
            }]' >&3
    fi

    logInfo "waiting for ingress-nginx to be ready"
    # wait for ingress-nginx
    kubectl wait jobs "ingress-nginx-admission-patch" \
        --namespace ingress-nginx \
        --for condition=complete \
        --timeout 120s >&3

    kubectl wait pods \
        --namespace ingress-nginx \
        --for condition=ready \
        --selector app.kubernetes.io/component=controller \
        --timeout 120s >&3

    logDebug "applying Custom Resource Definitions: Certificate, Issuer, and Bundle"
    kubectl apply \
        --filename https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.crds.yaml >&3
    kubectl apply \
        --filename https://raw.githubusercontent.com/cert-manager/trust-manager/v0.4.0/deploy/crds/trust.cert-manager.io_bundles.yaml >&3

    declare -A HELM_REPOS
    HELM_REPOS[openziti]="openziti.io/helm-charts"
    HELM_REPOS[jetstack]="charts.jetstack.io"
    HELM_REPOS[ingress-nginx]="kubernetes.github.io/ingress-nginx"
    for REPO in "${!HELM_REPOS[@]}"; do
        if helm repo list | cut -f1 | grep -qE "^${REPO}(\s+)?$"; then
            logDebug "refreshing ${REPO} Helm Charts"
            helm repo update "${REPO}" >&3
        else
            logInfo "subscribing to ${REPO} Helm Charts"
            helm repo add "${REPO}" "https://${HELM_REPOS[${REPO}]}" >&3
        fi
    done

    #
    ## Ensure OpenZiti Controller is Upgraded and Ready
    #

    if helm status ziti-controller --namespace "${ZITI_NAMESPACE}" &>/dev/null; then
        logInfo "upgrading openziti controller"
        helm upgrade "ziti-controller" "${ZITI_CHARTS}/ziti-controller" \
            --namespace "${ZITI_NAMESPACE}" \
            --set clientApi.advertisedHost="miniziti-controller.${MINIZITI_INGRESS_ZONE}" \
            --set trust-manager.app.trust.namespace="${ZITI_NAMESPACE}" \
            --set trust-manager.enabled=true \
            --set cert-manager.enabled=true \
            --values "${ZITI_CHARTS:-https://openziti.io/helm-charts/charts}/ziti-controller/values-ingress-nginx.yaml" >&3
    else
        logInfo "installing openziti controller"
        helm dependency build "${ZITI_CHARTS}/ziti-controller" >&3
        helm install "ziti-controller" "${ZITI_CHARTS}/ziti-controller" \
            --namespace "${ZITI_NAMESPACE}" --create-namespace \
            --set clientApi.advertisedHost="miniziti-controller.${MINIZITI_INGRESS_ZONE}" \
            --set trust-manager.app.trust.namespace="${ZITI_NAMESPACE}" \
            --set trust-manager.enabled=true \
            --set cert-manager.enabled=true \
            --values "${ZITI_CHARTS:-https://openziti.io/helm-charts/charts}/ziti-controller/values-ingress-nginx.yaml" >&3
    fi

    logDebug "setting default namespace '${ZITI_NAMESPACE}' in kubeconfig context '${MINIKUBE_PROFILE}'"
    minikube kubectl --profile "${MINIKUBE_PROFILE}" -- \
        config set-context "${MINIKUBE_PROFILE}" \
            --namespace "${ZITI_NAMESPACE}" >&3

    for DEPLOYMENT in ziti-controller-cert-manager trust-manager ziti-controller; do
        logInfo "waiting for $DEPLOYMENT to be ready"
        kubectl wait deployments "$DEPLOYMENT" \
            --namespace "${ZITI_NAMESPACE}" \
            --for condition=Available=True \
            --timeout 240s >&3
    done

    #
    ## Ensure Minikube Tunnel is Running on macOS and WSL
    #

    # wait to probe for the minikube tunnel until after controller deployment so there's at least one
    # ingress causing minikube to immediately prompt for sudo password
    if  (( DEBUG_MINIKUBE_TUNNEL )) || [[ "${DETECTED_OS}" =~ Windows|macOS ]]; then
        logDebug "detected OS is ${DETECTED_OS}, probing for minikube tunnel"
        if ! pgrep -f "minikube --profile ${MINIKUBE_PROFILE} tunnel" >&3; then
            echo -e "ERROR: ${DETECTED_OS} OS requires a running minikube tunnel for ingresses."\
                    " In another terminal, run the following command. Then re-run this script."\
                    "\n\n\tminikube --profile ${MINIKUBE_PROFILE} tunnel\n" >&2
            exit 1
        else
            logDebug "minikube tunnel is running"
        fi
        # recommend /etc/hosts change unless dns is configured to reach the minikube node IP
        checkDns "127.0.0.1"
    else
        checkDns "$MINIKUBE_NODE_EXTERNAL"
    fi

    #
    ## Ensure Cluster DNS is Resolving miniziti-controller.${MINIZITI_INGRESS_ZONE}
    #

    if ! testClusterDns "${MINIKUBE_NODE_EXTERNAL}" 2>/dev/null; then
        logDebug "initial cluster dns test failed, doing cluster dns setup"

        # xargs trims whitespace because minikube ssh returns a stray trailing '\r' after remote command output
        logDebug "probing minikube node for internal host record"
        MINIKUBE_NODE_INTERNAL=$(minikube --profile "${MINIKUBE_PROFILE}" ssh 'grep host.minikube.internal /etc/hosts')

        if [[ -n "${MINIKUBE_NODE_INTERNAL:-}" ]]; then
            # strip surrounding whitespace
            MINIKUBE_NODE_INTERNAL=$(xargs <<< "${MINIKUBE_NODE_INTERNAL}")
            logDebug "the minikube internal host record is \"${MINIKUBE_NODE_INTERNAL}\""
        else
            logError "failed to find minikube internal IP"
            exit 1
        fi

        (( MINIZITI_HOSTS )) && {
            logDebug "patching coredns configmap with *.${MINIZITI_INGRESS_ZONE} forwarder to minikube ingress-dns nameserver"
            kubectl patch configmap "coredns" \
                --namespace kube-system \
                --patch "
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
                ${MINIZITI_INGRESS_ZONE}:53 {
                    errors
                    cache 30
                    forward . ${MINIKUBE_NODE_EXTERNAL}
                }
        " >&3

            logDebug "deleting coredns pod so a new one will have modified Corefile"
            kubectl get pods \
                --namespace kube-system \
                | awk '/^coredns-/ {print $1}' \
                | xargs kubectl delete pods \
                    --namespace kube-system >&3

            logDebug "waiting for cluster dns to be ready"
            kubectl wait deployments "coredns" \
                --namespace kube-system \
                --for condition=Available=True >&3
        }

        # perform a DNS query in a pod so we know ingress-dns is working inside the cluster
        testClusterDns "${MINIKUBE_NODE_EXTERNAL}"
    fi

    #
    ## Ensure OpenZiti Router is Enrolled and Ready
    #

    logDebug "fetching admin password from k8s secret to log in to ziti mgmt"
    kubectl get secrets "ziti-controller-admin-secret" \
        --namespace "${ZITI_NAMESPACE}" \
        --output go-template='{{index .data "admin-password" | base64decode }}' \
        | xargs ziti edge login "miniziti-controller.${MINIZITI_INGRESS_ZONE}:443" \
            --yes --username "admin" \
            --password >&3

    if  ziti edge list edge-routers 'name="miniziti-router"' \
        | grep -q miniziti-router; then
        logDebug "updating miniziti-router"
        ziti edge update edge-router "miniziti-router" \
            --role-attributes "public-routers" >&3
    else
        logDebug "creating miniziti-router"
        ziti edge create edge-router "miniziti-router" \
            --role-attributes "public-routers" \
            --tunneler-enabled \
            --jwt-output-file /tmp/miniziti-router.jwt >&3
    fi

    if  helm status ziti-router --namespace "${ZITI_NAMESPACE}" &>/dev/null; then
        logDebug "upgrading router chart as 'ziti-router'"
        helm upgrade "ziti-router" "${ZITI_CHARTS}/ziti-router" \
            --namespace "${ZITI_NAMESPACE}" \
            --set enrollmentJwt=\ \
            --set edge.advertisedHost="miniziti-router.${MINIZITI_INGRESS_ZONE}" \
            --set linkListeners.transport.advertisedHost="miniziti-router-transport.${MINIZITI_INGRESS_ZONE}" \
            --set "ctrl.endpoint=ziti-controller-ctrl.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS:-https://openziti.io/helm-charts/charts}/ziti-router/values-ingress-nginx.yaml" >&3
    else
        logDebug "installing router chart as 'ziti-router'"
        helm dependency build "${ZITI_CHARTS}/ziti-router" >&3
        helm install "ziti-router" "${ZITI_CHARTS}/ziti-router" \
            --namespace "${ZITI_NAMESPACE}" \
            --set-file enrollmentJwt=/tmp/miniziti-router.jwt \
            --set edge.advertisedHost="miniziti-router.${MINIZITI_INGRESS_ZONE}" \
            --set linkListeners.transport.advertisedHost="miniziti-router-transport.${MINIZITI_INGRESS_ZONE}" \
            --set "ctrl.endpoint=ziti-controller-ctrl.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS:-https://openziti.io/helm-charts/charts}/ziti-router/values-ingress-nginx.yaml" >&3
    fi

    logInfo "waiting for ziti-router to be ready"
    kubectl wait deployments "ziti-router" \
        --namespace "${ZITI_NAMESPACE}" \
        --for condition=Available=True >&3

    logDebug "probing miniziti-router for online status"
    if ziti edge list edge-routers 'name="miniziti-router"' \
        | awk '/miniziti-router/ {print $6}' \
        | grep -q true; then
        logInfo "miniziti-router is online"
    else
        logError "miniziti-router is offline"
        exit 1
    fi

    #
    ## Ensure OpenZiti Console is Configured and Ready
    #

    if  helm --namespace "${ZITI_NAMESPACE}" list --all \
        | grep -q ziti-console; then
        logDebug "upgrading console chart as 'ziti-console'"
        helm upgrade "ziti-console" "${ZITI_CHARTS}/ziti-console" \
            --namespace "${ZITI_NAMESPACE}" \
            --set ingress.advertisedHost="miniziti-console.${MINIZITI_INGRESS_ZONE}" \
            --set "settings.edgeControllers[0].url=https://ziti-controller-client.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS:-https://openziti.io/helm-charts/charts}/ziti-console/values-ingress-nginx.yaml" >&3
    else
        logDebug "installing console chart as 'ziti-console'"
        helm dependency build "${ZITI_CHARTS}/ziti-console" >&3
        helm install "ziti-console" "${ZITI_CHARTS}/ziti-console" \
            --namespace "${ZITI_NAMESPACE}" \
            --set ingress.advertisedHost="miniziti-console.${MINIZITI_INGRESS_ZONE}" \
            --set "settings.edgeControllers[0].url=https://ziti-controller-client.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS:-https://openziti.io/helm-charts/charts}/ziti-console/values-ingress-nginx.yaml" >&3
    fi

    logInfo "waiting for ziti-console to be ready"
    kubectl wait deployments "ziti-console" \
        --namespace "${ZITI_NAMESPACE}" \
        --for condition=Available=True \
        --timeout 240s >&3

    logDebug "setting default namespace to '${ZITI_NAMESPACE}' in kubeconfig context '${MINIKUBE_PROFILE}'"
    minikube kubectl --profile "${MINIKUBE_PROFILE}" -- \
        config set-context "${MINIKUBE_PROFILE}" \
            --namespace "${ZITI_NAMESPACE}" >&3

    #
    ## Ensure OpenZiti Identities and Services are Created
    #

    if  ! ziti edge list identities "name=\"${MINIKUBE_PROFILE}-client\"" --csv \
        | grep -q "${MINIKUBE_PROFILE}-client"; then
        logDebug "creating identity ${MINIKUBE_PROFILE}-client"
        ziti edge create identity device "${MINIKUBE_PROFILE}-client" \
            --jwt-output-file "/tmp/${MINIKUBE_PROFILE}-client.jwt" --role-attributes httpbin-clients >&3
    else
        logDebug "ignoring identity ${MINIKUBE_PROFILE}-client"
    fi

    if  ! ziti edge list identities 'name="httpbin-host"' --csv \
        | grep -q "httpbin-host"; then
        logDebug "creating identity httpbin-host"
        ziti edge create identity device "httpbin-host" \
            --jwt-output-file /tmp/httpbin-host.jwt --role-attributes httpbin-hosts >&3
    else
        logDebug "ignoring identity httpbin-host"
    fi

    if  ! ziti edge list configs 'name="httpbin-intercept-config"' --csv \
        | grep -q "httpbin-intercept-config"; then
        logDebug "creating config httpbin-intercept-config"
        ziti edge create config "httpbin-intercept-config" intercept.v1 \
            '{"protocols":["tcp"],"addresses":["httpbin.'"${MINIZITI_INTERCEPT_ZONE}"'"], "portRanges":[{"low":80, "high":80}]}' >&3
    else
        logDebug "ignoring config httpbin-intercept-config"
    fi

    if  ! ziti edge list configs 'name="httpbin-host-config"' --csv \
        | grep -q "httpbin-host-config"; then
        logDebug "creating config httpbin-host-config"
        ziti edge create config "httpbin-host-config" host.v1 \
            '{"protocol":"tcp", "address":"httpbin","port":8080}' >&3
    else
        logDebug "ignoring config httpbin-host-config"
    fi

    if  ! ziti edge list services 'name="httpbin-service"' --csv \
        | grep -q "httpbin-service"; then
        logDebug "creating service httpbin-service"
        ziti edge create service "httpbin-service" \
            --configs httpbin-intercept-config,httpbin-host-config >&3
    else
        logDebug "ignoring service httpbin-service"
    fi

    if  ! ziti edge list service-policies 'name="httpbin-bind-policy"' --csv \
        | grep -q "httpbin-bind-policy"; then
        logDebug "creating service-policy httpbin-bind-policy"
        ziti edge create service-policy "httpbin-bind-policy" Bind \
            --service-roles '@httpbin-service' --identity-roles '#httpbin-hosts' >&3
    else
        logDebug "ignoring service-policy httpbin-bind-policy"
    fi

    if  ! ziti edge list service-policies 'name="httpbin-dial-policy"' --csv \
        | grep -q "httpbin-dial-policy"; then
        logDebug "creating service-policy httpbin-dial-policy"
        ziti edge create service-policy "httpbin-dial-policy" Dial \
            --service-roles '@httpbin-service' --identity-roles '#httpbin-clients' >&3
    else
        logDebug "ignoring service-policy httpbin-dial-policy"
    fi

    if  ! ziti edge list edge-router-policies 'name="public-routers"' --csv \
        | grep -q "public-routers"; then
        logDebug "creating edge-router-policy public-routers"
        ziti edge create edge-router-policy "public-routers" \
            --edge-router-roles '#public-routers' --identity-roles '#all' >&3
    else
        logDebug "ignoring edge-router-policy public-routers"
    fi

    if  ! ziti edge list service-edge-router-policies 'name="public-routers"' --csv \
        | grep -q "public-routers"; then
        logDebug "creating service-edge-router-policy public-routers"
        ziti edge create service-edge-router-policy "public-routers" \
            --edge-router-roles '#public-routers' --service-roles '#all' >&3
    else
        logDebug "ignoring service-edge-router-policy public-routers"
    fi

    if [[ -s /tmp/httpbin-host.jwt ]]; then
        logDebug "enrolling /tmp/httpbin-host.jwt"
        # discard expected output that normally flows to stderr
        ENROLL_OUT="$(
            ziti edge enroll /tmp/httpbin-host.jwt 2>&1 \
                | grep -vE '(generating.*key|enrolled\s+successfully)' \
                || true
        )"
        if [[ -z "${ENROLL_OUT}" ]]; then
            rm -f /tmp/httpbin-host.jwt
            logDebug "deleted /tmp/httpbin-host.jwt after enrolling successfully"
        else
            echo -e "ERROR: unexpected result during OpenZiti Identity enrollment\n"\
                    "${ENROLL_OUT}"
            exit 1
        fi
    fi

    if [[ -s /tmp/httpbin-host.json ]]; then
        logDebug "installing httpbin chart as 'miniziti-httpbin'"
        helm dependency build "${ZITI_CHARTS}/httpbin" >&3
        helm install "miniziti-httpbin" "${ZITI_CHARTS}/httpbin" \
            --set-file zitiIdentity=/tmp/httpbin-host.json \
            --set zitiServiceName=httpbin-service >&3
        rm -f /tmp/httpbin-host.json
        logDebug "deleted /tmp/httpbin-host.json after installing successfully with miniziti-httpbin chart"
    fi

    kubectl get secrets "ziti-controller-admin-secret" \
        --namespace "${ZITI_NAMESPACE}" \
        --output go-template='{{"\n'\
'INFO: Your OpenZiti Console is here:\thttps://miniziti-console.'"${MINIZITI_INGRESS_ZONE}"'\n'\
'INFO: The password for \"admin\" is:\t"}}{{index .data "admin-password" | base64decode }}'\
'{{"\n\n"}}'

    logInfo "Success! Remember to add your edge client identity '$(getClientOttPath "${DETECTED_OS}")' in your tunneler, e.g. Ziti Desktop Edge."
}

main "$@"
