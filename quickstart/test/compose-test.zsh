#!/usr/bin/env zsh 
#
# this script tests the quickstart's ziti-cli-functions.sh, container image creation process, and Compose project by
# gathering files from a particular GitHub repo ref or a filesystem path and running the quickstart's Go test suite
# against the running Compose project
#

set -euo pipefail

function down_project() {
    # don't destroy volumes or temp dir so we can inspect when running locally
    docker compose kill
    # rm -rf "${TESTDIR}"
    echo "INFO: Stopped Compose project: ${TESTDIR}"
}

DATESTAMP=$(date +%Y%m%d%H%M%S)
# generate a random password for the controller's admin user to ensure we're testing the expected instance
ZITI_PWD="$(set +o pipefail; LC_ALL=C tr -dc -- -A-Z-a-z-0-9 < /dev/urandom 2>/dev/null | head -c5)"
BASENAME="$(basename "$0")"
DIRNAME="$(dirname "$0")"
if [[ -n "${ZITI_QUICK_DIR:-}" ]]; then
    if [[ -d "${ZITI_QUICK_DIR}" ]]; then
        ZITI_QUICK_DIR="$(realpath "${ZITI_QUICK_DIR}")"
    else
        if [[ -d "${DIRNAME}/${ZITI_QUICK_DIR}" ]]; then
            ZITI_QUICK_DIR="$(realpath "${DIRNAME}/${ZITI_QUICK_DIR}")"
        else
            echo "ERROR: ZITI_QUICK_DIR is set but is not a directory: ${ZITI_QUICK_DIR}" >&2
            exit 1
        fi
    fi
fi
# avoid re-using directories from previous runs to keep this one-shot (non-idempotent) script simple because we needn't
# consider the state of the test dir
TESTDIR="$(mktemp -d -t "${BASENAME%.*}.${DATESTAMP}.XXX")"

# if unset, set ZITI_QUICK_DIR to this script's parent dir which is always the quickstart root in the git repo
if [[ -z "${ZITI_QUICK_DIR:-}" ]]; then
    ZITI_QUICK_DIR="$(realpath "${DIRNAME}/..")"
fi
# if unset, set ZITI_QUICK_IMAGE_TAG to this run's dirname
if [[ -z "${ZITI_QUICK_IMAGE_TAG:-}" ]]; then
    ZITI_QUICK_IMAGE_TAG=$(basename "${TESTDIR}")
fi

# case "${1:-}" in
#         shift
#         ;;
#     --help|-h)
#         echo "Usage: $BASENAME [--local|--help]"
#         exit 0
#         ;;
# esac

cd "${TESTDIR}"
echo "INFO: Testing Compose project $PWD"

declare -a QUICK_FILES=(
    ../go.{mod,sum}
    test/compose.override.yml
    docker/{simplified-docker-compose.yml,.env}
)
        # TODO: re-add cert checks files after https://github.com/openziti/ziti/pull/1278
    # test/{quickstart_test.go,compose.override.yml,check-cert-chains.zsh}
# download the quickstart Go test suite files from GitHub unless a local dir is specified
if [[ -n "${ZITI_QUICK_DIR:-}" ]]; then
    for FILE in "${QUICK_FILES[@]}"; do
        cp "${ZITI_QUICK_DIR}/${FILE}" .
    done
    if [[ -n "${ZITI_QUICK_IMAGE_TAG:-}" ]]; then
        if [[ -x "${ZITI_QUICK_DIR:-}/docker/createLocalImage.sh" ]]; then
            (
                cd "${ZITI_QUICK_DIR}/docker"
                unset ZITI_VERSION ZITI_OVERRIDE_VERSION  # always build the local source
                ./createLocalImage.sh --build "${ZITI_QUICK_IMAGE_TAG}"
            )
        else
            echo "ERROR: ZITI_QUICK_IMAGE_TAG is set but ZITI_QUICK_DIR/docker/createLocalImage.sh is not executable" >&2
            exit 1
        fi
    fi
elif [[ -n "${ZITI_QUICK_IMAGE_TAG:-}" ]]; then
    echo "ERROR: ZITI_QUICK_IMAGE_TAG is set but ZITI_QUICK_DIR is not set" >&2
    exit 1
else
    echo "ERROR: ZITI_QUICK_IMAGE_TAG is not set, try running with --local" >&2
    exit 1
fi

# rename the simplified Compose file to the default Compose project file name
mv ./simplified-docker-compose.yml ./compose.yml

# learn the expected Go version from the Go mod file so we can pull the correct container image
ZITI_GO_VERSION="$(awk '/^go[[:space:]]+/ {print $2}' ./go.mod)"
# make this var available in the Compose project
sed -E \
    -e  "s/^(#[[:space:]]+)?(ZITI_PWD)=.*/\2=${ZITI_PWD}/" \
    -e  "s/^(#[[:space:]]+)?(ZITI_INTERFACE)=.*/\2=${ZITI_INTERFACE:-127.0.0.1}/" ./.env > ./.env.tmp
mv ./.env.tmp ./.env

# pull images preemptively that we never build locally because pull=never when using a local quickstart image
for IMAGE in \
    "golang:${ZITI_GO_VERSION}-alpine" \
    "openziti/zac:latest"
do
    docker pull --quiet "${IMAGE}" &>/dev/null
done

# any halt after this point should cause the Compose project to be torn down
trap down_project SIGTERM SIGINT EXIT

# these compose vars are used to configure the golang service that runs the test suite
echo -e "ZITI_GO_VERSION=${ZITI_GO_VERSION}"\
        "\nGOPATH=${GOPATH:-${HOME}/go}"\
        "\nZITI_QUICK_DIR=${ZITI_QUICK_DIR}" \
        >> ./.env

# if ZITI_QUICK_IMAGE_TAG is set then run the locally-built image
if [[ -n "${ZITI_QUICK_IMAGE_TAG:-}" ]]; then
    sed -Ee "s/^(#[[:space:]]+)?(ZITI_VERSION)=.*/\2=${ZITI_QUICK_IMAGE_TAG}/" ./.env > ./.env.tmp
    mv ./.env.tmp ./.env
    docker compose up --detach --pull=never &>/dev/null # no pull because local quickstart image
else
    echo "ERROR: ZITI_QUICK_IMAGE_TAG is not set" >&2
    exit 1
fi

# copy files that are not present in older quickstart container images to the persistent volume; this allows us to run
# the test suite against them and investigate if the test fails and the container is destroyed
# for FILE in \
    # ""
    # check-cert-chains.zsh
    # TODO: re-add cert checks to cp list after https://github.com/openziti/ziti/pull/1278
# do
    # docker compose cp \
    #     "./${FILE}" \
    #     "ziti-controller:/persistent/${FILE}" &>/dev/null
# done
# TODO: build these executables into the container image?

# wait for the controller and router to be ready and run the certificate check script; NOUNSET option is enabled after
# sourcing quickstart functions and env because there are some unset variables in those
docker compose exec ziti-controller \
    bash -eo pipefail -c '
        source "${ZITI_SCRIPTS}/ziti-cli-functions.sh" >/dev/null;
        echo "INFO: waiting for controller";
        source /persistent/ziti.env >/dev/null;
        _wait_for_controller >/dev/null;
        echo "INFO: waiting for public router";
        source /persistent/ziti.env >/dev/null;
        _wait_for_public_router >/dev/null;
    '
        # TODO: re-add cert checks to above test suite after https://github.com/openziti/ziti/pull/1278
        # zsh /persistent/check-cert-chains.zsh;
docker compose run quickstart-test

echo -e "\nINFO: Test completed successfully."

