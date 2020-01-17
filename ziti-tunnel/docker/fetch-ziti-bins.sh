#!/bin/bash -eu
set -o pipefail

if [ -n "${ARTIFACTORY_TOKEN}" ]; then
    echo "Fetching from Artifactory."
    if [ -z "${ARTIFACTORY_BASE_URL}" ]; then ARTIFACTORY_BASE_URL="https://netfoundry.jfrog.io/netfoundry"; fi
    if [ -z "${ARTIFACTORY_REPO}" ]; then  ARTIFACTORY_REPO="ziti-release"; fi
    for var in ARTIFACTORY_BASE_URL ARTIFACTORY_REPO ZITI_VERSION; do
        if [ -z "${!var}" ]; then
            echo "ERROR: ${var} must be set when fetching binaries from Artifactory." >&2
            exit 1
        fi
    done

    # map host architecture/os to directories that we use in netfoundry.jfrog.io.
    # (our artifact directories seem to align with Docker's TARGETARCH and TARGETOS
    #  build arguments, which we could rely on if we used buildkit - see
    #  https://docs.docker.com/engine/reference/builder/#automatic-platform-args-in-the-global-scope)
    host_arch=$(uname -m)
    case "${host_arch}" in
    "x86_64") artifact_arch="amd64";;
    "armv7l") artifact_arch="arm";;
    *) echo "ERROR: ziti binaries do not exist for architecture ${host_arch}"; exit 1;;
    esac

    host_os=$(uname -s)
    case "${host_os}" in
    "Linux") artifact_os="linux";;
    "Darwin") artifact_os="darwin";;
    "Windows") artifact_os="windows";;
    *) echo "ERROR: ziti binaries do not exist for os ${host_os}"; exit 1;;
    esac

    for exe in "${@}"; do
      url="${ARTIFACTORY_BASE_URL}/${ARTIFACTORY_REPO}/${exe}/${artifact_arch}/${artifact_os}/${ZITI_VERSION}/${exe}.tar.gz"
      echo "Fetching ${exe} from ${url}"
      rm -f "${exe}" "${exe}.tar.gz" "${exe}.exe"
      if which curl 2>&1 >/dev/null; then
        curl -H "X-JFrog-Art-Api:${ARTIFACTORY_TOKEN}" -fLsS -O "${url}"
      elif which wget 2>&1 >/dev/null; then
        wget --header "X-JFrog-Art-Api:${ARTIFACTORY_TOKEN}" "${url}"
      else
        echo "ERROR: need one of curl or wget to fetch the artifact." >&2
        exit 1
      fi
      tar -xzf "${exe}.tar.gz"
      rm "${exe}.tar.gz"
    done
else
    echo "ERROR: The var ARTIFACTORY_TOKEN must be defined for authentication with HTTP header X-JFrog-Art-Api." >&2
    exit 1
fi