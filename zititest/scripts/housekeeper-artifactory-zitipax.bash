#!/usr/in/env bash

set -euo pipefail

BASENAME=$(basename "$0")
typeset -a  ARTIFACTS=() \
            STAGES=() \
            KNOWN_ARTIFACTS=(openziti openziti-controller openziti-router openziti-console zrok zrok-share ziti-edge-tunnel)
typeset -A KNOWN_STAGES=(
    [testing]='zitipax-(openziti-(rpm|deb)-test|fork-(rpm|deb)-stable)'
    [release]='zitipax-openziti-(rpm|deb)-stable'
)

: "${AGE:=30}"  # days
: "${CI:=0}"    # jfrog CLI is interactive and prompts for confirmation by default
export CI

while (( $# )); do
    case ${1} in
        --artifacts)
            shift
            while [[ $# -gt 0 && ! ${1} =~ ^-- ]]; do
                ARTIFACTS+=("$1")
                shift
            done
            for ARTIFACT in "${ARTIFACTS[@]}"; do
                for KNOWN in "${KNOWN_ARTIFACTS[@]}"; do
                    if [[ ${ARTIFACT} == "${KNOWN}" ]]; then
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
                        continue 2
                    fi
                done
                echo "ERROR: invalid stage '${STAGE}', valid stages are ${!KNOWN_STAGES[*]}" >&2
                exit 1
            done
            echo "INFO: stages are ${STAGES[*]}" >&2
            ;;
        --quiet)
            CI=1  # disable interactive prompts before destructive actions
            shift
            ;;
        --dry-run)
            CI=0  # re-enable interactive prompts before destructive actions in case parent env has CI=1
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
        --help|\?|*)
            echo "Usage: $BASENAME --artifacts openziti ziti-edge-tunnel [--age DAYS|--stages testing release|--dry-run|--quiet|--glob GLOB]"
            exit 0
            ;;
    esac
done

if [[ ${#ARTIFACTS[@]} -eq 0 ]]; then
    echo "ERROR: no artifacts specified, need one or more of ${KNOWN_ARTIFACTS[*]}" >&2
    exit 1
else
    echo "INFO: artifacts are ${ARTIFACTS[*]}" >&2
fi

if [[ ${#STAGES[@]} -eq 0 ]]; then
    echo "INFO: default stage is 'testing'" >&2
    STAGES=(testing)
fi

(( CI )) && {
    echo "WARNING: permanently deleting" >&2;
    sleep 9;
}

for STAGE in "${STAGES[@]}"; do
    while read -r REPO; do
        for ARTIFACT in "${ARTIFACTS[@]}"; do
            for META in rpm.metadata.name deb.name; do
                    echo "INFO: deleting ${REPO}/${ARTIFACT}" >&2
                    jf rt search --include 'created;path' --props "${META}=${ARTIFACT}" "${REPO}/${GLOB:-*}" \
                    | jq --arg OLDEST "$(date --date "-${AGE} days" -Is)" '.[]|select(.created < $OLDEST)|.path' \
                    | xargs -rl jf rt delete
            done
        done
    done < <(
        jf rt cl -sS /api/repositories \
        | jq --raw-output --arg repo_regex "${KNOWN_STAGES[$STAGE]}" '.[]|select(.key|match($repo_regex))|.key'
    )
done
