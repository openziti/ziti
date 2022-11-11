if [[ -f "runonce" ]]; then
    echo "Keeping active for testing"
    tail -f /dev/null
fi

touch runonce

source "${ZITI_SCRIPTS}/ziti-cli-functions.sh"
expressInstall "${ZITI_NETWORK}"