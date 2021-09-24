#!/bin/bash

set -euo pipefail

[[ $# -eq 0 ]] && {
    echo "ERROR: need the base name(s) of the executable(s) to fetch e.g. \"ziti-tunnel ...\"." >&2
    exit 1
}

: "${ARTIFACTORY_BASE_URL:="https://netfoundry.jfrog.io/netfoundry"}"
: "${ARTIFACTORY_REPO:="ziti-release"}"
for var in ARTIFACTORY_BASE_URL ARTIFACTORY_REPO ZITI_VERSION; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: ${var} must be set when fetching binaries from Artifactory." >&2
        exit 1
    fi
done

[[ "$ZITI_VERSION" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]] || {
    echo "ERROR: '$ZITI_VERSION' is not a semantic version like 2.0.0" >&2
    exit 1
}

echo "Fetching from Artifactory."

# map host architecture/os to directories that we use in netfoundry.jfrog.io.
# (our artifact directories seem to align with Docker's TARGETARCH and TARGETOS
#  build arguments, which we could rely on if we fully committed to "docker buildx" - see
#  https://docs.docker.com/engine/reference/builder/#automatic-platform-args-in-the-global-scope)
host_arch=$(uname -m)
case "${host_arch}" in
"x86_64") artifact_arch="amd64";;
"armv7l"|"aarch64") artifact_arch="arm";;
*) echo "ERROR: ziti binaries do not exist for architecture ${host_arch}"; exit 1;;
esac

host_os=$(uname -s)
case "${host_os}" in
    "Linux") artifact_os="linux"
    ;;
    "Darwin") artifact_os="darwin"
    ;;
    "Windows") artifact_os="windows"
    ;;
    *)  echo "ERROR: ziti binaries do not exist for os ${host_os}"; 
        exit 1
    ;;
esac

for exe in "${@}"; do
    url="${ARTIFACTORY_BASE_URL}/${ARTIFACTORY_REPO}/${exe}/${artifact_arch}/${artifact_os}/${ZITI_VERSION}/${exe}.tar.gz"
    echo "Fetching ${exe} from ${url}"
    rm -f "${exe}" "${exe}.tar.gz" "${exe}.exe"
    token_header="${ARTIFACTORY_TOKEN:+X-JFrog-Art-Api:$ARTIFACTORY_TOKEN}"
    if { command -v curl > /dev/null; } 2>&1; then
        curl ${token_header:+-H "$token_header"} -fLsS -O "${url}"
    elif { command -v wget > /dev/null; } 2>&1; then
        wget ${token_header:+--header "$token_header"} "${url}"
    else
        echo "ERROR: need one of curl or wget to fetch the artifact." >&2
        exit 1
    fi
    tar -xzf "${exe}.tar.gz"
    if [ -f "${exe}" ]; then chmod 755 "${exe}"; fi
    if [ -f "${exe}.exe" ]; then chmod 755 "${exe}.exe"; fi
    rm "${exe}.tar.gz"
done
