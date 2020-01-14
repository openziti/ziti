#!/bin/bash -e

if [ -n "${ARTIFACTORY_USERNAME}" ]; then
    echo "ARTIFACTORY_USERNAME=${ARTIFACTORY_USERNAME}. Assuming fetch from artifactory."
    if [ -z "${ARTIFACTORY_BASE_URL}" ]; then ARTIFACTORY_BASE_URL="https://netfoundry.jfrog.io/netfoundry"; fi
    if [ -z "${ARTIFACTORY_REPO}" ]; then  ARTIFACTORY_REPO="ziti-release"; fi
    for var in ARTIFACTORY_TOKEN ARTIFACTORY_BASE_URL ARTIFACTORY_REPO ZITI_VERSION; do
        if [ -z "${!var}" ]; then
            echo "ERROR: ${var} must be set when fetching binaries from artifactory."
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
      curl -u "${ARTIFACTORY_USERNAME}:${ARTIFACTORY_TOKEN}" -fLsS -O "${url}"
      tar -xzf "${exe}.tar.gz"
      rm "${exe}.tar.gz"
    done

fi