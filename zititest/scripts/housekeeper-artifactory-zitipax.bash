#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

function _usage {
cat <<USAGE

Usage: $BASENAME --artifacts ARTIFACT... [OPTIONS]

Prompts for confirmation before deleting each selected artifact unless --quiet.

Required:
  --artifacts ARTIFACT...  One or more artifacts to process
                          Valid artifacts: ${KNOWN_ARTIFACTS[*]}

Options:
  --age DAYS              Match artifacts older than DAYS days (default: ${AGE})
  --stages STAGE...       Search repositories from the pre-release or stable sets, or both (default: ${STAGES[*]})
                          Valid stages: ${!KNOWN_STAGES[*]}
  --version VERSION       Narrow selection glob-match artifact version (example: 3.2.1* matches 3.2.1-2 and 3.2.1~234)
  --glob GLOB             Narrow selection by glob-match artifact name (default: ${GLOB})
  --dry-run               Disable communication with Artifactory
  --quiet                 Skip the delete confirmation prompt
  --help                  Show this help message

Example:
  $BASENAME --artifacts openziti ziti-edge-tunnel --age 60 --stages testing
USAGE
    
}

BASENAME=$(basename "$0")
typeset -a  ARTIFACTS=() \
            STAGES=() \
            KNOWN_ARTIFACTS=(openziti openziti-controller openziti-router openziti-console zrok zrok-share zrok-agent ziti-edge-tunnel)
typeset -A KNOWN_STAGES=(
    [testing]='zitipax-(openziti-(rpm|deb)-test|fork-(rpm|deb)-stable)'
    [release]='zitipax-openziti-(rpm|deb)-stable'
)

DRY_RUN=''
VERSION=''
: "${AGE:=0}"  # days
: "${CI:=0}"    # jfrog CLI is interactive and prompts for confirmation by default
: "${QUIET:=0}"
: "${STAGES:=testing}"
: "${GLOB:=*}"

export CI

while (( $# )); do
    case ${1} in
        --help|-h)
            _usage
            exit 0
            ;;
        --artifacts)
            shift
            while [[ $# -gt 0 && ! ${1} =~ ^-- ]]; do
                ARTIFACTS+=("$1")
                shift
            done
            for ARTIFACT in "${ARTIFACTS[@]}"; do
                for KNOWN in "${KNOWN_ARTIFACTS[@]}"; do
                    if [[ ${ARTIFACT} == "${KNOWN}" ]]; then
                        # De-duplicate the list before continuing
                        ARTIFACTS=($(printf '%s\n' "${ARTIFACTS[@]}" | sort -u))
                        continue 2
                    fi
                done
                echo "ERROR: invalid artifact '${ARTIFACT}', valid artifacts are ${KNOWN_ARTIFACTS[*]}" >&2
                exit 1
            done
            ;;
        --stages)
            shift
            while [[ $# -gt 0 && ! ${1} =~ ^-- ]]; do
                STAGES+=("$1")
                shift
            done
            for STAGE in "${STAGES[@]}"; do
                for KNOWN in "${!KNOWN_STAGES[@]}"; do
                    if [[ ${STAGE} == "${KNOWN}" ]]; then
                        # De-duplicate the list before continuing
                        STAGES=($(printf '%s\n' "${STAGES[@]}" | sort -u))
                        continue 2
                    fi
                done
                echo "ERROR: invalid stage '${STAGE}', valid stages are ${!KNOWN_STAGES[*]}" >&2
                exit 1
            done
            echo "INFO: operating on repos from stage(s): ${STAGES[*]}" >&2
            ;;
        --quiet)  # suppress jfrog CLI interactive prompts
            QUIET=1
            CI=1
            shift
            ;;
        --dry-run)  # disable communication with Artifactory
            DRY_RUN=1
            shift
            ;;
        --age)
            shift
            if [[ ${1} =~ ^-- ]]; then
                echo "ERROR: --age DAYS requires an integer argument" >&2
                exit 1
            else
                AGE="$1"
                shift
            fi
            ;;
        --glob)
            shift
            if [[ ${1} =~ ^-- ]]; then
                echo "ERROR: --glob GLOB requires a string argument" >&2
                exit 1
            else
                GLOB="$1"
                shift
            fi
            ;;
        --version)
            shift
            if [[ ${1} =~ ^-- ]]; then
                echo "ERROR: --version VERSION requires a string argument" >&2
                exit 1
            else
                VERSION="$1"
                shift
            fi
            ;;
        \?|*)
            _usage
            exit 0
            ;;
    esac
done

if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
    echo "ERROR: no artifacts specified, need one or more of ${KNOWN_ARTIFACTS[*]}" >&2
    exit 1
else
    echo "INFO: searching for artifacts: ${ARTIFACTS[*]}" >&2
fi

if (( CI )) && ! (( DRY_RUN )); then
    echo "WARNING: pausing for 10s before permanently deleting the selected artifacts, press Ctrl-C to abort" >&2;
    sleep 10;
fi

for STAGE in "${STAGES[@]}"; do
    while read -r REPO; do
        for ARTIFACT in "${ARTIFACTS[@]}"; do
            echo "INFO: deleting ${REPO}/${ARTIFACT} matching '${GLOB}'" >&2
            for META in rpm.metadata.name,rpm.metadata.version deb.name,deb.version; do
                _meta_name=${META%%,*}
                _meta="${_meta_name}=${ARTIFACT}"
                if [[ -n ${VERSION} ]]; then
                    _meta_version=${META#*,}
                    _meta+=";${_meta_version}=${VERSION}"
                fi
                jf rt search --include 'created;path' --props "${_meta}" "${REPO}/${GLOB}" \
                | jq --arg OLDEST "$(date --utc --date "-${AGE} days" -Is)" '.[]|select(.created < $OLDEST)|.path' \
                | xargs --no-run-if-empty --max-lines=1 --verbose --open-tty jf rt delete ${DRY_RUN:+--dry-run}
            done
        done
    done < <(
        jf rt cl -sS /api/repositories \
        | jq --raw-output --arg repo_regex "${KNOWN_STAGES[$STAGE]}" '.[]|select(.key|match($repo_regex))|.key'
    )
done
