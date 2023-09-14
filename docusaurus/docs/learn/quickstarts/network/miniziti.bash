#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

checkBashVersion() {
    if (( "${BASH_VERSION%%.*}" < 4 )); then
        echo "This script requires Bash major version 4 or greater."
        echo "Detected version: $BASH_VERSION"
        if [[ ${OSTYPE:-} =~ [Dd]arwin ]]; then
            echo -e "\nOn macOS, you can install bash with Homebrew:"
            echo "brew install bash"
            echo -e "\nThen run:"
            #shellcheck disable=SC2016
            echo '"$(brew --prefix bash)" ./miniziti.bash ...'
        fi
        exit 1;
    fi
}

banner(){

    local profile="${1-}"

    cat <<BANNER
                  __   ___
    |\/| | |\ | |  / |  |  |
    |  | | | \| | /_ |  |  |

BANNER

    if [[ -n "$profile" && "$profile" != "miniziti" ]]; then
        echo -e "   .: $* :.\n"
    fi
}

_usage(){
    if (( $# )); then
        echo -e "\nERROR: unexpected arg '$1'" >&2
    else
        banner
    fi
    echo -e "\n Basic Commands:\n"\
            "   start\t\tstart miniziti\n"\
            "   delete\t\tdelete miniziti\n"\
            "   console\t\topen ziti-console in browser\n"\
            "   creds\t\tprints admin user updb credentials\n"\
            "   status\t\tprint ziti component status\n"\
            "   ziti\t\tziti cli wrapper\n"\
            "   help\t\tshow these usage hints\n"\
            "\n Profile Commands:\n"\
            "   profile list\tlist available profiles\n"\
            "   profile show\tshow default profile\n"\
            "   profile use\t\tset default profile\n"\
            "\n Advanced Commands:\n"\
            "   shell\t\trun interactive shell inside the ziti-controller container\n"\
            "   login\t\trun local ziti binary edge login\n"\
            "\n Other Commands:\n"\
            "   kubectl\t\tkubectl cli wrapper\n"\
            "   minikube\t\tminikube cli wrapper\n"\
            "\n Options:\n"\
            "   --quiet\t\tsuppress INFO messages\n"\
            "   --verbose\t\tshow DEBUG messages\n"\
            "   --profile\t\tMINIKUBE_PROFILE (miniziti)\n"\
            "   --namespace\t\tZITI_NAMESPACE (MINIKUBE_PROFILE)\n"\
            "   --no-hosts\t\tdon't use local hosts DB or ingress-dns nameserver\n"\
            "   --modify-hosts\tadd entries to local hosts database. Requires sudo if not running as root. Linux only.\n"\
            "\n Debug:\n"\
            "   --charts\t\tZITI_CHARTS_REF (openziti) alternative charts repo\n"\
            "   --now\t\teliminate safety waits, e.g., before deleting miniziti\n"\
            "   --\t\t\tMINIKUBE_START_ARGS args after -- passed to minikube start\n"
}

_usageProfileUse(){
    echo -e "\nSet miniziti default profile\n"\
            "\n Usage:\n"\
            "   ziti profile use [profile]\n"
}

_usageProfile(){
    if (( $# )); then
        echo -e "\nERROR: unexpected arg '$1'" >&2
    fi

    echo -e "\nProfile management commands\n"\
            "\n Usage:\n"\
            "   ziti profile [command]\n"\
            "\n Available Commands:\n"\
            "   list\tlist available profiles\n"\
            "   show\tshow default profile\n"\
            "   use\t\tset miniziti profile\n"
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

makeMinizitiStateDir() {
    case "$DETECTED_OS" in
        macOS)
            state_dir="${XDG_STATE_DIR:-$HOME/Library/Application Support}/miniziti"
            ;;
        Linux|Windows)
            state_dir="${XDG_STATE_DIR:-$HOME/.local/state}/miniziti"
            ;;
        *)
            logError "Unknown os: $DETECTED_OS"
            exit 1
    esac

    if [[ ! -d "$state_dir" ]]; then
        logDebug "Creating miniziti state directory: $state_dir"
        mkdir -p "$state_dir"
    fi

    echo "$state_dir"
}

getZitiCliHome() {
    case "$DETECTED_OS" in
        Linux) echo "$HOME/.config/ziti" ;;
        *) echo "$HOME/.ziti" ;;
    esac
}

pathToNative() {
    local path="$1"
    case "$DETECTED_OS" in
        Windows) wslpath -w "$path" ;;
        *) echo "$path" ;;
    esac
}

testClusterDns(){
    if kubectlWrapper run "dnstest" --rm --tty --stdin --image busybox --restart Never -- \
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

checkSudoRequired() {
    if (( EUID != 0 )); then
        logInfo "sudo is required when not running as root"
    fi
}

HOSTS_FILE='/etc/hosts'

cleanHosts() {
    local marker="$1"
    logWarn "Removing stale miniziti entries from $HOSTS_FILE"
    if (( EUID != 0 )); then
        sudo sed -i "/$marker/d" "$HOSTS_FILE"
    else
        sed -i "/$marker/d" "$HOSTS_FILE"
    fi
}

installHosts() {
    hosts=(
        "miniziti-controller.$MINIZITI_INGRESS_ZONE"
        "miniziti-router.$MINIZITI_INGRESS_ZONE"
        "miniziti-console.$MINIZITI_INGRESS_ZONE"
    )

    hosts_line="$MINIKUBE_NODE_EXTERNAL ${hosts[*]}"

    if ! grep -q "$hosts_line" "$HOSTS_FILE"; then

        checkSudoRequired

        if grep -q "$MINIZITI_INGRESS_ZONE" "$HOSTS_FILE"; then
            cleanHosts "$MINIZITI_INGRESS_ZONE"
        fi

        logInfo "Adding miniziti entries to $HOSTS_FILE"
        if (( EUID != 0 )); then
            echo "$hosts_line" | sudo tee -a "$HOSTS_FILE" > /dev/null
        else
            echo "$hosts_line" | tee -a "$HOSTS_FILE" > /dev/null
        fi
    fi
}

getAdminSecret() {
    kubectlWrapper get secrets "ziti-controller-admin-secret" \
        --namespace "$ZITI_NAMESPACE" \
        --output go-template='{{index .data "admin-password" | base64decode }}'
}

showAdminCreds() {
    logInfo "The password for 'admin' is: '$(getAdminSecret)'"
}

getConfigMapKey() {
    local configmap="$1"
    local key="$2"
    kubectlWrapper get configmap "$configmap" \
        --ignore-not-found=true \
        --output jsonpath="{.data.$key}" 2> /dev/null
}

getIngressZone() {
    ingress_zone="$(getConfigMapKey "$MINIZITI_CONFIGMAP" 'ingress-zone')"
    if [[ -z "$ingress_zone" ]]; then
        logError "Failed to retrieve ingress zone. Did you start profile '$MINIKUBE_PROFILE' successfully?"
        exit 1
    else
        echo "$ingress_zone"
    fi
}

minizitiLogin() {
    checkCommand ziti
    ingress_zone="$(getIngressZone)"
    getAdminSecret | xargs ziti edge login "https://miniziti-controller.$ingress_zone:443/edge/management/v1" \
            --cli-identity "$MINIKUBE_PROFILE" \
            --yes \
            --username "admin" \
            --password

    logInfo "Setting default ziti identity to: $MINIKUBE_PROFILE"
    zitiWrapper edge use "$MINIKUBE_PROFILE" >&3
}

minizitiConsole() {
    ingress_zone="$(getIngressZone)"
    console_url="https://miniziti-console.$ingress_zone"
    case "$DETECTED_OS" in
        "Windows")
            checkCommand wslview
            wslview "$console_url"
        ;;
        "macOS")
            checkCommand open
            open "$console_url"
        ;;
        *)
            checkCommand xdg-open
            xdg-open "$console_url"
        ;;
    esac
    logInfo "Opening ziti-console: $console_url ..."
}

listProfiles() {
    dir --format=single-column --ignore=default --sort=time --reverse "$PROFILES_DIR"
}

logger() {
    local caller="${FUNCNAME[1]}"

    if (( $# < 1 )); then
        echo "ERROR: $caller() takes 1 or more args" >&2
        return 1
    fi

    local message="$*"

    if [[ "$message" =~ ^r\'(.+)\'$ ]]; then
        raw_message="${BASH_REMATCH[1]}"
        message="$raw_message"
    fi

    caller_level="${caller##log}"
    if (( MINIZITI_DEBUG )); then
        line="${caller_level^^} ${FUNCNAME[2]}:${BASH_LINENO[1]}: $message"
    else
        line="${caller_level^^} $message"
    fi

    if [[ -n "${raw_message-}" ]]; then
        echo -E "$line"
    else
        echo -e "$line"
    fi
}

logInfo() {
    logger "$*"
}

logWarn() {
    logger "$*" >&2
}

logError() {
    logger "$*" >&2
}

logDebug() {
    logger "$*" >&3
}

controllerPod() {
    kubectlWrapper get pods \
        --selector app.kubernetes.io/component=ziti-controller \
        --output jsonpath='{.items[0].metadata.name}'
}

getPodStatus() {
    local pod_name="$1"
    kubectlWrapper get pod \
        --selector app.kubernetes.io/component="$pod_name" \
        --ignore-not-found=true \
        --output jsonpath='{.items[0].status.phase}' 2> /dev/null
}

showStatus() {
    COMPONENTS=("ziti-controller" "ziti-router" "ziti-console")
    for component in "${COMPONENTS[@]}"; do
        status="$(getPodStatus "$component")"
        if [[ -z "$status" ]]; then
            logError "Could not retrieve status for component $component"
            exit 1
        fi
        echo "$component: $status"
    done
}

getDefaultProfile() {
    default_profile="$DEFAULT_PROFILE"
    if [[ -L "$default_profile" ]]; then
        basename "$(readlink -f "$default_profile")"
    else
        echo "miniziti"
    fi
}

setProfile() {
    profile="$1"
    profiles="$(listProfiles)"
    if ! echo "$profiles" | grep -q "$profile"; then
        logError "Invalid profile: $profile\nValid profiles are:\n$profiles"
        exit 1
    else
        ln --no-dereference --symbolic --force "$PROFILES_DIR/$profile" "$DEFAULT_PROFILE"
        logInfo "Set default profile to: '$profile'"
    fi
}

zitiWrapper() {
    kubectlWrapper exec "$(controllerPod)" --container ziti-controller -- zitiLogin > /dev/null
    kubectlWrapper exec "$(controllerPod)" --container ziti-controller -- ziti "$@"
}

shellWrapper() {
    kubectlWrapper exec "$(controllerPod)" --container ziti-controller --tty --stdin -- bash
}

kubectlWrapper() {
    minikube kubectl --profile "$MINIKUBE_PROFILE" -- --context "$MINIKUBE_PROFILE" "$@"
}

minikubeWrapper() {
    minikube --profile "$MINIKUBE_PROFILE" "$@"
}

helmWrapper() {
    helm --kube-context "$MINIKUBE_PROFILE" "$@"
}

checkCommand() {
    if ! command -v "$1" &>/dev/null; then
        logError "this script requires command '$1'. Please install on the search PATH and try again."
        $1
    fi
}

main(){
    checkBashVersion >&2
    MINIZITI_DEBUG=0
    # require commands
    declare -a BINS=(awk grep helm jq minikube nslookup pgrep sed xargs)
    for BIN in "${BINS[@]}"; do
        checkCommand "$BIN"
    done

    # open a descriptor for debug messages
    exec 3>/dev/null

    # local strings with defaults that never produce an error
    declare DELETE_MINIZITI=0 \
            DETECTED_OS \
            DO_ZITI_LOGIN=0 \
            MINIKUBE_NODE_EXTERNAL \
            MINIKUBE_PROFILE \
            MINIZITI_HOSTS=1 \
            MINIZITI_MODIFY_HOSTS=0 \
            OPEN_ZITI_CONSOLE=0 \
            RUN_ZITI_CLI=0 \
            SAFETY_WAIT=1 \
            SHOW_ADMIN_CREDS=0 \
            START_MINIZITI=0 \
            ZITI_CHARTS_ALT=0 \
            ZITI_CHARTS_REF="openziti" \
            ZITI_CHARTS_URL="https://openziti.io/helm-charts/charts" \
            ZITI_NAMESPACE

    # local arrays with defaults that never produce an error
    declare -a MINIKUBE_START_ARGS=()

    # local defaults that are inherited or may error
    DETECTED_OS="$(detectOs)"
    : "${DEBUG_MINIKUBE_TUNNEL:=0}"  # set env = 1 to trigger the minikube tunnel probe
    : "${MINIZITI_TIMEOUT_SECS:=240}"
    MINIZITI_CONFIGMAP="miniziti-config"
    ZITI_CLI_HOME="$(getZitiCliHome)"
    ZITI_CLI_CERTS_DIR="$ZITI_CLI_HOME/certs"
    STATE_DIR="$(makeMinizitiStateDir)"
    PROFILES_DIR="$STATE_DIR/profiles"
    [[ ! -d "$PROFILES_DIR" ]] && mkdir "$PROFILES_DIR"
    DEFAULT_PROFILE="$PROFILES_DIR/default"

    MINIKUBE_PROFILE="$(getDefaultProfile)"

    while (( $# )); do
        case "$1" in
            start)          START_MINIZITI=1
                            shift
            ;;
            delete)         DELETE_MINIZITI=1
                            shift
            ;;
            console)        OPEN_ZITI_CONSOLE=1
                            shift
            ;;
            creds)          SHOW_ADMIN_CREDS=1
                            shift
            ;;
            profile)
                            shift
                            if (( $# == 0 )); then
                                _usageProfile
                                exit 1
                            fi
                            case "$1" in
                                list)
                                    shift
                                    listProfiles
                                    exit
                                ;;
                                use)
                                    shift
                                    if (( $# != 1 )); then
                                        _usageProfileUse
                                        exit 1
                                    fi
                                    setProfile "$1"
                                    exit
                                ;;
                                show)
                                    getDefaultProfile
                                    exit
                                ;;
                                *)
                                    _usageProfile "$1"
                                    exit 1
                                ;;
                            esac
            ;;
            login)          DO_ZITI_LOGIN=1
                            shift
            ;;
            status)
                            showStatus
                            exit
            ;;
            ziti)           RUN_ZITI_CLI=1
                            shift
                            ziti_cli_args=("$@")
                            shift "${#ziti_cli_args[@]}"
            ;;
            kubectl)        shift
                            kubectlWrapper "${@:-}"
                            exit
            ;;
            minikube)       shift
                            minikubeWrapper "${@:-}"
                            exit
            ;;
            shell)          shift
                            shellWrapper "${@:-}"
                            exit
            ;;
            -p|--profile)   validateDnsName "$2"
                            MINIKUBE_PROFILE="$2"
                            # sanity check the profile name input
                            if [[ ${MINIKUBE_PROFILE} =~ ^- ]]; then
                                # in case the arg value is another option instead of the profile name
                                logError "--profile needs a profile name not starting with a hyphen"
                                _usage
                                exit 1
                            fi

                            shift 2
            ;;
            -n|--namespace) ZITI_NAMESPACE="$2"
                            shift 2
            ;;
            --charts)       ZITI_CHARTS_REF="$2"
                            ZITI_CHARTS_URL="$2"
                            ZITI_CHARTS_ALT=1
                            shift 2
            ;;
            -q|--quiet)     exec > /dev/null
                            shift
            ;;
            -v|--verbose|--debug)
                            MINIZITI_DEBUG=1
                            exec 3>&1
                            shift
            ;;
            --now)          SAFETY_WAIT=0
                            shift
            ;;
            --no-hosts)     MINIZITI_HOSTS=0
                            shift
            ;;
            --modify-hosts) if [[ "$DETECTED_OS" != "Linux" ]]; then
                                logError "The '--modify-hosts' option is only available for Linux"
                                exit 1
                            fi
                            MINIZITI_MODIFY_HOSTS=1
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

    : "${ZITI_NAMESPACE:=${MINIKUBE_PROFILE}}"

    MINIZITI_INGRESS_ZONE="$MINIKUBE_PROFILE.internal"
    MINIZITI_INTERCEPT_ZONE="$MINIKUBE_PROFILE.private"
    PROFILE_DIR="$PROFILES_DIR/${MINIKUBE_PROFILE}"
    IDENTITIES_DIR="$PROFILE_DIR/identities"

    if (( DO_ZITI_LOGIN )); then
        minizitiLogin
        exit 0
    fi

    if (( SHOW_ADMIN_CREDS )); then
        showAdminCreds
        exit 0
    fi

    if (( OPEN_ZITI_CONSOLE )); then
        minizitiConsole
        exit 0
    fi

    if (( RUN_ZITI_CLI )); then
        zitiWrapper  "${ziti_cli_args[@]}"
        exit 0
    fi

    if (( DELETE_MINIZITI )); then
        banner "$MINIKUBE_PROFILE"

        trap EXIT
        if ingress_zone="$(getIngressZone)"; then
            if (( MINIZITI_MODIFY_HOSTS )) && grep -q "$ingress_zone" "$HOSTS_FILE"; then
                checkSudoRequired
                cleanHosts "$ingress_zone"
            fi

            CERT_FILE="$(find "$ZITI_CLI_CERTS_DIR" -maxdepth 1 -type f -name "miniziti-controller.$ingress_zone" -print -quit 2> /dev/null)"
            if [[ -n "$CERT_FILE" ]]; then
                logWarn "Deleting miniziti certificate file: $CERT_FILE"
                rm -f  "$CERT_FILE"
            fi
        fi
        trap - EXIT

        deleteMiniziti 10

        if [[ -d "$PROFILE_DIR" ]]; then
            logWarn "Deleting miniziti profile directory: $PROFILE_DIR"
            rm -rf  "$PROFILE_DIR"
        fi

        if [[ -L "$DEFAULT_PROFILE" ]]; then
            if [[ "$(basename "$(readlink -f "$DEFAULT_PROFILE")")" == "$MINIKUBE_PROFILE" ]]; then
                unlink "$DEFAULT_PROFILE"
            fi
        fi

        # Cannot nicely call logout until https://github.com/openziti/ziti/issues/1305 is addressed.
        # if checkCommand ziti &>/dev/null; then
        #     logWarn "Removing $MINIKUBE_PROFILE profile identity from ziti-cli.json"
        #     ziti edge logout --cli-identity "$MINIKUBE_PROFILE" >&3
        # fi

        exit 0
    fi

    if (( START_MINIZITI != 1 )); then
        _usage
        exit 0;
    fi


    banner "$MINIKUBE_PROFILE"

    if [[ ! -d "$IDENTITIES_DIR" ]]; then
        logDebug "Creating miniziti identities directory: ($IDENTITIES_DIR)"
        mkdir -p "$IDENTITIES_DIR"
    fi

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
        MINIZITI_INGRESS_ZONE="${MINIKUBE_NODE_EXTERNAL}.sslip.io"
        logDebug "DNS wildcard zone for ingresses is ${MINIZITI_INGRESS_ZONE}"
    }

    if [[ -n "${MINIKUBE_NODE_EXTERNAL:-}" ]]; then
        logDebug "the minikube external IP is ${MINIKUBE_NODE_EXTERNAL}"
    else
        logError "failed to find minikube external IP"
        exit 1
    fi

    if (( MINIZITI_MODIFY_HOSTS )); then
        installHosts
    fi

    # verify current context can connect to apiserver
    kubectlWrapper cluster-info >&3
    logDebug "kubectl successfully obtained cluster-info from apiserver"

    # enable ssl-passthrough for OpenZiti ingresses
    if kubectlWrapper get deployment "ingress-nginx-controller" \
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
        kubectlWrapper patch deployment "ingress-nginx-controller" \
            --namespace ingress-nginx \
            --type json \
            --patch '[{"op": "add",
                "path": "/spec/template/spec/containers/0/args/-",
                "value":"--enable-ssl-passthrough"
            }]' >&3
    fi

    logInfo "waiting for ingress-nginx to be ready"
    # wait for ingress-nginx
    kubectlWrapper wait jobs "ingress-nginx-admission-patch" \
        --namespace ingress-nginx \
        --for condition=complete \
        --timeout "${MINIZITI_TIMEOUT_SECS}s" >&3

    kubectlWrapper wait pods \
        --namespace ingress-nginx \
        --for condition=ready \
        --selector app.kubernetes.io/component=controller \
        --timeout "${MINIZITI_TIMEOUT_SECS}s" >&3

    logDebug "applying Custom Resource Definitions: Certificate, Issuer, and Bundle"
    kubectlWrapper apply \
        --filename https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.crds.yaml >&3
    kubectlWrapper apply \
        --filename https://raw.githubusercontent.com/cert-manager/trust-manager/v0.4.0/deploy/crds/trust.cert-manager.io_bundles.yaml >&3

    declare -A HELM_REPOS
    HELM_REPOS[openziti]="openziti.io/helm-charts"
    HELM_REPOS[jetstack]="charts.jetstack.io"
    HELM_REPOS[ingress-nginx]="kubernetes.github.io/ingress-nginx"
    for REPO in "${!HELM_REPOS[@]}"; do
        if helmWrapper repo list | cut -f1 | grep -qE "^${REPO}(\s+)?$"; then
            logDebug "refreshing ${REPO} Helm Charts"
            helmWrapper repo update "${REPO}" >&3
        else
            logInfo "subscribing to ${REPO} Helm Charts"
            helmWrapper repo add "${REPO}" "https://${HELM_REPOS[${REPO}]}" >&3
        fi
    done

    #
    ## Ensure OpenZiti Controller is Upgraded and Ready
    #

    if helmWrapper status ziti-controller --namespace "${ZITI_NAMESPACE}" &>/dev/null; then
        logInfo "upgrading openziti controller"
        helmWrapper upgrade "ziti-controller" "${ZITI_CHARTS_REF}/ziti-controller" \
            --namespace "${ZITI_NAMESPACE}" \
            --set clientApi.advertisedHost="miniziti-controller.${MINIZITI_INGRESS_ZONE}" \
            --set trust-manager.app.trust.namespace="${ZITI_NAMESPACE}" \
            --set trust-manager.enabled=true \
            --set cert-manager.enabled=true \
            --values "${ZITI_CHARTS_URL}/ziti-controller/values-ingress-nginx.yaml" >&3
    else
        logInfo "installing openziti controller"
        (( ZITI_CHARTS_ALT )) && {
            helmWrapper dependency build "${ZITI_CHARTS_REF}/ziti-controller" >&3
        }
        helmWrapper install "ziti-controller" "${ZITI_CHARTS_REF}/ziti-controller" \
            --namespace "${ZITI_NAMESPACE}" --create-namespace \
            --set clientApi.advertisedHost="miniziti-controller.${MINIZITI_INGRESS_ZONE}" \
            --set trust-manager.app.trust.namespace="${ZITI_NAMESPACE}" \
            --set trust-manager.enabled=true \
            --set cert-manager.enabled=true \
            --values "${ZITI_CHARTS_URL}/ziti-controller/values-ingress-nginx.yaml" >&3
    fi

    logDebug "setting default namespace '${ZITI_NAMESPACE}' in kubeconfig context '${MINIKUBE_PROFILE}'"
        kubectlWrapper config set-context "${MINIKUBE_PROFILE}" \
            --namespace "${ZITI_NAMESPACE}" >&3

    for DEPLOYMENT in ziti-controller-cert-manager trust-manager ziti-controller; do
        logInfo "waiting for $DEPLOYMENT to be ready"
        kubectlWrapper wait deployments "$DEPLOYMENT" \
            --namespace "${ZITI_NAMESPACE}" \
            --for condition=Available=True \
            --timeout "${MINIZITI_TIMEOUT_SECS}s" >&3
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
            kubectlWrapper patch configmap "coredns" \
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
            kubectlWrapper delete pods \
                --context "$MINIKUBE_PROFILE" \
                --namespace kube-system \
                --selector k8s-app=kube-dns >&3

            logDebug "waiting for cluster dns to be ready"
            kubectlWrapper wait deployments "coredns" \
                --namespace kube-system \
                --for condition=Available=True \
                --timeout "${MINIZITI_TIMEOUT_SECS}s" >&3
        }

        # perform a DNS query in a pod so we know ingress-dns is working inside the cluster
        testClusterDns "${MINIKUBE_NODE_EXTERNAL}"
    fi

    if kubectlWrapper get configmap "$MINIZITI_CONFIGMAP" 2> /dev/null; then
        logDebug "$MINIZITI_CONFIGMAP configmap has been applied"
    else
        logInfo "Applying $MINIZITI_CONFIGMAP configmap"
        cat <<EOF | kubectlWrapper apply -f - >&3
apiVersion: v1
kind: ConfigMap
metadata:
  name: "$MINIZITI_CONFIGMAP"
  namespace: "$ZITI_NAMESPACE"
data:
  ingress-zone: "$MINIZITI_INGRESS_ZONE"
EOF
    fi

    #
    ## Ensure OpenZiti Router is Enrolled and Ready
    #
    #

    logInfo "Setting default ziti identity to: $MINIKUBE_PROFILE"
    zitiWrapper edge use "$MINIKUBE_PROFILE" >&3

    ROUTER_NAME='miniziti-router'
    ROUTER_OTT="$IDENTITIES_DIR/$ROUTER_NAME.jwt"
    if  zitiWrapper edge list edge-routers "name=\"$ROUTER_NAME\"" \
        | grep -q miniziti-router; then
        logDebug "updating $ROUTER_NAME"
        zitiWrapper edge update edge-router "$ROUTER_NAME" \
            --role-attributes "public-routers" >&3
    else
        logDebug "creating $ROUTER_NAME"
        zitiWrapper edge create edge-router "$ROUTER_NAME" \
            --role-attributes "public-routers" \
            --tunneler-enabled >&3
        zitiWrapper edge list edge-routers \
            "name=\"$ROUTER_NAME\"" \
            --output-json \
            | jq --exit-status --raw-output '.data[0].enrollmentJwt' > "$ROUTER_OTT"
    fi

    if  helmWrapper status ziti-router --namespace "${ZITI_NAMESPACE}" &>/dev/null; then
        logDebug "upgrading router chart as 'ziti-router'"
        helmWrapper upgrade "ziti-router" "${ZITI_CHARTS_REF}/ziti-router" \
            --namespace "${ZITI_NAMESPACE}" \
            --set enrollmentJwt=\ \
            --set edge.advertisedHost="miniziti-router.${MINIZITI_INGRESS_ZONE}" \
            --set linkListeners.transport.advertisedHost="miniziti-router-transport.${MINIZITI_INGRESS_ZONE}" \
            --set "ctrl.endpoint=ziti-controller-ctrl.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS_URL}/ziti-router/values-ingress-nginx.yaml" >&3
    else
        logDebug "installing router chart as 'ziti-router'"
        (( ZITI_CHARTS_ALT )) && {
            helmWrapper dependency build "${ZITI_CHARTS_REF}/ziti-router" >&3
        }
        helmWrapper install "ziti-router" "${ZITI_CHARTS_REF}/ziti-router" \
            --namespace "${ZITI_NAMESPACE}" \
            --set-file enrollmentJwt="$ROUTER_OTT" \
            --set edge.advertisedHost="miniziti-router.${MINIZITI_INGRESS_ZONE}" \
            --set linkListeners.transport.advertisedHost="miniziti-router-transport.${MINIZITI_INGRESS_ZONE}" \
            --set "ctrl.endpoint=ziti-controller-ctrl.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS_URL}/ziti-router/values-ingress-nginx.yaml" >&3
    fi

    logInfo "waiting for ziti-router to be ready"
    kubectlWrapper wait deployments "ziti-router" \
        --namespace "${ZITI_NAMESPACE}" \
        --for condition=Available=True \
        --timeout "${MINIZITI_TIMEOUT_SECS}s" >&3

    logDebug "probing miniziti-router for online status"
    if zitiWrapper edge list edge-routers "name=\"$ROUTER_NAME\"" \
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

    if  helmWrapper --namespace "${ZITI_NAMESPACE}" list --all \
        | grep -q ziti-console; then
        logDebug "upgrading console chart as 'ziti-console'"
        helmWrapper upgrade "ziti-console" "${ZITI_CHARTS_REF}/ziti-console" \
            --namespace "${ZITI_NAMESPACE}" \
            --set ingress.advertisedHost="miniziti-console.${MINIZITI_INGRESS_ZONE}" \
            --set "settings.edgeControllers[0].url=https://ziti-controller-client.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS_URL}/ziti-console/values-ingress-nginx.yaml" >&3
    else
        logDebug "installing console chart as 'ziti-console'"
        (( ZITI_CHARTS_ALT )) && {
            helmWrapper dependency build "${ZITI_CHARTS_REF}/ziti-console" >&3
        }
        helmWrapper install "ziti-console" "${ZITI_CHARTS_REF}/ziti-console" \
            --namespace "${ZITI_NAMESPACE}" \
            --set ingress.advertisedHost="miniziti-console.${MINIZITI_INGRESS_ZONE}" \
            --set "settings.edgeControllers[0].url=https://ziti-controller-client.${ZITI_NAMESPACE}.svc:443" \
            --values "${ZITI_CHARTS_URL}/ziti-console/values-ingress-nginx.yaml" >&3
    fi

    logInfo "waiting for ziti-console to be ready"
    kubectlWrapper wait deployments "ziti-console" \
        --namespace "${ZITI_NAMESPACE}" \
        --for condition=Available=True \
        --timeout "${MINIZITI_TIMEOUT_SECS}s" >&3

    logDebug "setting default namespace to '${ZITI_NAMESPACE}' in kubeconfig context '${MINIKUBE_PROFILE}'"
        kubectlWrapper config set-context "${MINIKUBE_PROFILE}" \
            --namespace "${ZITI_NAMESPACE}" >&3

    #
    ## Ensure OpenZiti Identities and Services are Created
    #

    CLIENT_NAME="${MINIKUBE_PROFILE}-client"
    CLIENT_OTT="$IDENTITIES_DIR/$CLIENT_NAME.jwt"

    if  ! zitiWrapper edge list identities "name=\"$CLIENT_NAME\"" --csv \
        | grep -q "$CLIENT_NAME"; then
        logDebug "creating identity $CLIENT_NAME"
        zitiWrapper edge create identity "$CLIENT_NAME" \
            --role-attributes httpbin-clients >&3
        zitiWrapper edge list identities "name=\"$CLIENT_NAME\"" \
            --output-json \
        | jq --exit-status --raw-output '.data[0].enrollment.ott.jwt' > "$CLIENT_OTT"
    else
        logDebug "ignoring identity $CLIENT_NAME"
    fi

    HTTPBIN_NAME="httpbin-host"
    HTTPBIN_OTT="$IDENTITIES_DIR/$HTTPBIN_NAME.jwt"

    if  ! zitiWrapper edge list identities "name=\"$HTTPBIN_NAME\"" --csv \
        | grep -q "$HTTPBIN_NAME"; then
        logDebug "creating identity $HTTPBIN_NAME"
        zitiWrapper edge create identity "$HTTPBIN_NAME" \
            --role-attributes httpbin-hosts >&3
        zitiWrapper edge list identities "name=\"$HTTPBIN_NAME\"" \
            --output-json \
        | jq --exit-status --raw-output '.data[0].enrollment.ott.jwt' > "$HTTPBIN_OTT"

    else
        logDebug "ignoring identity $HTTPBIN_NAME"
    fi

    if  ! zitiWrapper edge list configs 'name="httpbin-intercept-config"' --csv \
        | grep -q "httpbin-intercept-config"; then
        logDebug "creating config httpbin-intercept-config"
        zitiWrapper edge create config "httpbin-intercept-config" intercept.v1 \
            '{"protocols":["tcp"],"addresses":["httpbin.'"${MINIZITI_INTERCEPT_ZONE}"'"], "portRanges":[{"low":80, "high":80}]}' >&3
    else
        logDebug "ignoring config httpbin-intercept-config"
    fi

    if  ! zitiWrapper edge list configs 'name="httpbin-host-config"' --csv \
        | grep -q "httpbin-host-config"; then
        logDebug "creating config httpbin-host-config"
        zitiWrapper edge create config "httpbin-host-config" host.v1 \
            '{"protocol":"tcp", "address":"httpbin","port":8080}' >&3
    else
        logDebug "ignoring config httpbin-host-config"
    fi

    if  ! zitiWrapper edge list services 'name="httpbin-service"' --csv \
        | grep -q "httpbin-service"; then
        logDebug "creating service httpbin-service"
        zitiWrapper edge create service "httpbin-service" \
            --configs httpbin-intercept-config,httpbin-host-config >&3
    else
        logDebug "ignoring service httpbin-service"
    fi

    if  ! zitiWrapper edge list service-policies 'name="httpbin-bind-policy"' --csv \
        | grep -q "httpbin-bind-policy"; then
        logDebug "creating service-policy httpbin-bind-policy"
        zitiWrapper edge create service-policy "httpbin-bind-policy" Bind \
            --service-roles '@httpbin-service' --identity-roles '#httpbin-hosts' >&3
    else
        logDebug "ignoring service-policy httpbin-bind-policy"
    fi

    if  ! zitiWrapper edge list service-policies 'name="httpbin-dial-policy"' --csv \
        | grep -q "httpbin-dial-policy"; then
        logDebug "creating service-policy httpbin-dial-policy"
        zitiWrapper edge create service-policy "httpbin-dial-policy" Dial \
            --service-roles '@httpbin-service' --identity-roles '#httpbin-clients' >&3
    else
        logDebug "ignoring service-policy httpbin-dial-policy"
    fi

    if  ! zitiWrapper edge list edge-router-policies 'name="public-routers"' --csv \
        | grep -q "public-routers"; then
        logDebug "creating edge-router-policy public-routers"
        zitiWrapper edge create edge-router-policy "public-routers" \
            --edge-router-roles '#public-routers' --identity-roles '#all' >&3
    else
        logDebug "ignoring edge-router-policy public-routers"
    fi

    if  ! zitiWrapper edge list service-edge-router-policies 'name="public-routers"' --csv \
        | grep -q "public-routers"; then
        logDebug "creating service-edge-router-policy public-routers"
        zitiWrapper edge create service-edge-router-policy "public-routers" \
            --edge-router-roles '#public-routers' --service-roles '#all' >&3
    else
        logDebug "ignoring service-edge-router-policy public-routers"
    fi

    if [[ -s "$HTTPBIN_OTT" ]]; then
        logDebug "installing httpbin chart as 'miniziti-httpbin'"
        (( ZITI_CHARTS_ALT )) && {
            helmWrapper dependency build "${ZITI_CHARTS_REF}/httpbin" >&3
        }
        helmWrapper install "miniziti-httpbin" "${ZITI_CHARTS_REF}/httpbin" \
            --set-file zitiEnrollment="$HTTPBIN_OTT" \
            --set zitiServiceName=httpbin-service >&3
        rm -f "$HTTPBIN_OTT"
        logDebug "deleted $HTTPBIN_OTT after installing successfully with miniziti-httpbin chart"
    fi

    echo -e "\n\n"
    logInfo "Your OpenZiti Console is here: https://miniziti-console.${MINIZITI_INGRESS_ZONE}"
    showAdminCreds
    echo -e "\n\n"

    logInfo "r'Success! Remember to add your edge client identity '$(pathToNative "$CLIENT_OTT")' in your tunneler, e.g. Ziti Desktop Edge.'"
}

main "$@"
