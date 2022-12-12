if [[ -f "runonce" ]]; then
    echo "Keeping active for testing"
    tail -f /dev/null
fi

touch runonce

source "${ZITI_SCRIPTS}/ziti-cli-functions.sh"
# Set the default password to be a specific value, if not set the password will be a random string and tests will fail
export ZITI_PWD=admin
expressInstall "localhost"