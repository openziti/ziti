#!/bin/bash
commands_to_test=(ziti ziti-router ziti-controller bash tee jq curl cat dos2unix)

if [[ "" == "$1" ]]
then
    echo " "
    echo "USAGE: $0 <path to ziti binaries> <network name>"
    echo " "
    echo "    Please provide the path to the ziti binaries"
    exit 1
else
    export PATH="$PATH:$1"
fi

# verify all the commands required in the automation exist before trying to run the full suite
for cmd in "${commands_to_test[@]}"
do
    # checking all commands are on the path before continuing...
    if ! [[ -x "$(command -v ${cmd})" ]]; then
        missing_requirements="${missing_requirements}    * ${cmd}\n"
    fi
done

# are requirements ? if yes, stop here and help 'em out
if ! [[ "" = "${missing_requirements}" ]]; then
    echo " "
    echo "The commands listed below are required to be on the path for this script to function properly."
    echo "Please ensure the commands listed are on the path and then try again."
    printf "\n${missing_requirements}"
    exit 1
fi

echo "All required commands are found - continuing"
echo "${missing_requirements}"

# set a variable to the location of the script running in case it's needed
export curdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

# if no network name is supplied, use the current user name as the network name
if [[ "" == "$2" ]]
then
    export network_name="${USER}"
else
    export network_name="$2"
fi
echo "Network name set to: ${network_name}"


. ${curdir}/env.sh

echo "This script relies on the following hosts being active on your network and since this is a local"
echo "installation it expects them all to be at localhost/127.0.0.1. Please make sure the following"
echo "entries are in your hosts file before continuing:"
echo "  - ${ZITI_CONTROLLER_HOSTNAME}"
echo "  - ${ZITI_EDGE_HOSTNAME}"
echo "  - ${ZITI_EDGE_ROUTER_HOSTNAME}"
echo "  - ${ZITI_ROUTER_BR_HOSTNAME}"
echo "  - ${ZITI_ROUTER_BLUE_HOSTNAME}"
echo "  - ${ZITI_ROUTER_RED_HOSTNAME}"
echo ""
echo "   example:"
echo "   127.0.0.1 ${ZITI_CONTROLLER_HOSTNAME} ${ZITI_EDGE_HOSTNAME} ${ZITI_EDGE_ROUTER_HOSTNAME} ${ZITI_ROUTER_BR_HOSTNAME} ${ZITI_ROUTER_BLUE_HOSTNAME} ${ZITI_ROUTER_RED_HOSTNAME}"
echo ""

while true; do
    read -p "Are the hosts in your hosts file? " yn
    case $yn in
        [Yy]* )
            break;;
        [Nn]* )
            echo ""; echo "Ok - come back when you're ready."
            exit;;
	* ) echo "Please answer yes or no. (yes/NO)";;
    esac
done

mkdir -p $ZITI_HOME
cd $ZITI_HOME


if [ -d "${ZITI_PKI}/${ZITI_CONTROLLER_ROOTCA_NAME}" ]
then
    echo "Reusing existing PKI...."
else
    echo "Generating PKI"
    . ${curdir}/create-pki.sh
fi

. ${curdir}/create-config-files.sh
. ${curdir}/prep-ziti.sh
. ${curdir}/start-ziti.sh
. ${curdir}/test-ziti.sh

echo "staring a new bash shell to retain all environment variables without polluting the initial shell"
bash --rcfile <(cat << HERE
. ~/.bashrc 
export PS1="ZITI IS RUNNING: "

echo "adding pki-functions to bash shell"
. ${curdir}/pki-functions.sh

alias zec='ziti edge controller'

HERE
)
