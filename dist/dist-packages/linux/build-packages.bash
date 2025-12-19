#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

exec 3>/dev/null             # suppress debug on 3
exec 4>&1; exec 1>/dev/null  # stash stdout on 4

declare -a ARTIFACTS=(openziti{-{controller,router},})
declare -a ARCHS=(amd64)
: "${TMPDIR:=$(mktemp -d)}"
: "${INSTALL:=false}"
: "${PUSH:=false}"
: "${DOCKER:=false}"
: "${CLEAN:=false}"
BASENAME=$(basename $0)
DIRNAME=$(dirname $0)
cd "${DIRNAME}/../../.."

while (( $# )); do
	case $1 in
		--artifacts)
			shift
			ARTIFACTS=()
			while (( $# )) && ! [[ $1 =~ ^-- ]]
			do
				if [[ $1 =~ ^openziti(-(controller|router))?$ ]]; then
					ARTIFACTS+=("$1")
					shift
				else
					echo "Unknown artifact: $1" >&2
					exit 1
				fi
			done
			;;
		--clean)
			CLEAN=true
			shift
			;;
		--docker)
			DOCKER=true
			shift
			;;
		--push)
			PUSH=true
			shift
			;;
		--install)
			INSTALL=true
			shift
			;;
		--verbose)
			exec 1>&4  # restore stdout from 4
			shift
			;;
		--debug)
			set -o xtrace
			exec 1>&4  # restore stdout from 4
			exec 3>&1  # voice debug from 3
			export DEBUG=1
			shift
			;;
		--help)
			exec 1>&4  # restore stdout from 4
			echo -e "Usage: ${BASENAME} FLAGS"\
				"\n\t--artifacts\tbuild space-separated - any of openziti, openziti-controller, openziti-router (default: all)"\
				"\n\t--install\tinstall Debian packages"\
				"\n\t--clean\t\tpurge Debian package files"\
				"\n\t--docker\tbuild docker images"\
				"\n\t--push\t\tpush docker images"\
				"\n\t--verbose\temit informational messages and chatty stdout"\
				"\n\t--debug\t\tdebug this script"
			exit
			;;
		*)
			echo "Unknown argument: $1"
			exit 1
			;;
	esac
done

# Track the explicitly requested artifacts. Some artifacts may be auto-added later
# (for example, openziti as a docker base image dependency).
declare -a EXPLICIT_ARTIFACTS=("${ARTIFACTS[@]}")

ARTIFACTS_DIR=./release
# export to nfpm and assign right after building ziti binary
export ZITI_VERSION ZITI_REV
: ${HUB_USER:=kbinghamnetfoundry}

 function buildZitiGoBuilder {
	# build the builder
	docker buildx build \
		--tag=ziti-go-builder \
		--build-arg uid="$UID" \
		--load \
		./dist/docker-images/cross-build/ 2>&3
	echo "INFO: Built ziti-go-builder"
}

function runZitiGoBuilder {
	# Detect go.work and add bind mounts for each 'use' path (other than '.')
	typeset -a GO_WORK_FILES=("$PWD/go.work" "$PWD/../go.work")
	GO_WORK_MOUNTS=""
	for W in ${GO_WORK_FILES[@]}
	do
		# use highest precedence workspace file
		if [[ -s "$W" ]]
		then
			while read -r path
			do
				path="${path//\"/}" # Remove quotes if any
				if [[ "$path" != "." && -n "$path" ]]
				then
					abs_path="$(realpath "$(dirname ${W})/$path")"
					base_path="$(basename "$path")"
					GO_WORK_MOUNTS+=" --volume=$abs_path:/mnt/$base_path"
				fi
			done < <(awk '/use *\(/, /\)/ { if ($1 != "use" && $1 != "(" && $1 != ")") print $1 }' "$W")
			if [[ "$(realpath ${W})" == "$(realpath ${PWD}/../go.work)" ]]
			then
				GO_WORK_MOUNTS+=" --volume=${W}:/mnt/go.work"
				GO_WORK_MOUNTS+=" --volume=${W}.sum:/mnt/go.work.sum"
			fi
			break
		fi
	done
	if ! grep -qE '\b/mnt/ziti\b' <<< "${GO_WORK_MOUNTS}"
	then
		GO_WORK_MOUNTS+=" --volume=$PWD:/mnt/ziti"
	fi
	docker run \
		--rm \
		--user "$UID" \
		--name=ziti-go-builder \
		${GO_WORK_MOUNTS} \
		--volume="${GOCACHE:-${HOME}/.cache/go-build}:/.cache/go-build" \
		${GOEXPERIMENT:+--env GOEXPERIMENT="${GOEXPERIMENT:-}"} \
		${GOFIPS140:+--env GOFIPS140="${GOFIPS140:-}"} \
		--env=TAGS \
		--env=GOCACHE=/.cache/go-build \
		ziti-go-builder "$1"
}

function setArtifactVars {
	case ${1} in
		openziti)
			ARTIFACT_SHORT=ziti-cli
			;;
		openziti-controller)
			ARTIFACT_SHORT=ziti-controller
			ZITI_ENV_FILE=/opt/openziti/etc/controller/bootstrap.env
			ZITI_CRED_FILE=/opt/openziti/etc/controller/.pwd
			ZITI_HOME=/var/lib/ziti-controller
			;;
		openziti-router)
			ARTIFACT_SHORT=ziti-router
			ZITI_ENV_FILE=/opt/openziti/etc/router/bootstrap.env
			ZITI_CRED_FILE=/opt/openziti/etc/router/.token
			ZITI_HOME=/var/lib/ziti-router
			;;
	esac
}

# Build order for packages/binaries:
# - If CLEAN is requested, clean dependent services before rebuilding the CLI.
# - Otherwise, ensure openziti (CLI) is built first so other artifacts can use the built
#   binary for versioning/packaging.
ARTIFACTS_BUILD=()
if [[ "${CLEAN}" == true ]]
then
	for ARTIFACT in "${ARTIFACTS[@]}"
	do
		if [[ "${ARTIFACT}" != openziti ]]
		then
			ARTIFACTS_BUILD+=("${ARTIFACT}")
		fi
	done
	for ARTIFACT in "${ARTIFACTS[@]}"
	do
		if [[ "${ARTIFACT}" == openziti ]]
		then
			ARTIFACTS_BUILD+=("${ARTIFACT}")
			break
		fi
	done
else
	for ARTIFACT in "${ARTIFACTS[@]}"
	do
		if [[ "${ARTIFACT}" == openziti ]]
		then
			ARTIFACTS_BUILD+=("${ARTIFACT}")
			break
		fi
	done
	for ARTIFACT in "${ARTIFACTS[@]}"
	do
		if [[ "${ARTIFACT}" != openziti ]]
		then
			ARTIFACTS_BUILD+=("${ARTIFACT}")
		fi
	done
fi

echo "DEBUG: ARTIFACTS_BUILD=${ARTIFACTS_BUILD[*]}" >&3
for ARTIFACT in "${ARTIFACTS_BUILD[@]}"
do

	setArtifactVars "$ARTIFACT"
	for ARCH in "${ARCHS[@]}"
	do
		if [[ ${ARTIFACT} == openziti ]]
		then
			buildZitiGoBuilder
			runZitiGoBuilder "$ARCH"
			echo "INFO: Built ${ARTIFACT} for ${ARCH}"
			cp -v $ARTIFACTS_DIR/$ARCH/linux/ziti $ARTIFACTS_DIR/ziti;
		fi

		ZITI_VERSION="$($ARTIFACTS_DIR/ziti --version)" || {
			echo "ERROR: Failed to get version from $ARTIFACTS_DIR/ziti" >&2
			echo "INFO: try building only artifact 'openziti' first" >&2
			exit 1
		}
		ZITI_REV="$(git rev-parse --short HEAD)"
		echo "DEBUG: Version ${ZITI_VERSION}-${ZITI_REV}" >&3

		for PKG in deb rpm
		do
			ZITI_HOMEPAGE="https://openziti.io" \
			ZITI_VENDOR=netfoundry \
			ZITI_MAINTAINER="Maintainers <developers@openziti.org>" \
			GOARCH=$ARCH \
			MINIMUM_SYSTEMD_VERSION=232 \
			nfpm pkg \
				--packager $PKG \
				--target  "$TMPDIR" \
				--config "./dist/dist-packages/linux/nfpm-${ARTIFACT}.yaml"
			echo "INFO: Built ${ARTIFACT} for ${ARCH} with ${PKG}"
		done
	done

	[[ "${CLEAN}" == true ]] && {
		if [[ "${ARTIFACT}" =~ openziti-(controller|router) ]]
		then
			for SVC in ${ARTIFACT_SHORT}.service
			do
				(set +e
					sudo systemctl stop "${SVC}"
					sudo systemctl disable --now "${SVC}"
					sudo systemctl reset-failed "${SVC}"
					sudo systemctl clean --what=state "${SVC}"
				)||true
				echo "INFO: Cleaned ${SVC}"
				if [[ -n "$(sudo ls -A "${ZITI_HOME:-}" 2>/dev/null)" ]]
				then
					sudo rm -rf "${ZITI_HOME:-}/"
					echo "INFO: Cleaned non-empty ZITI_HOME=${ZITI_HOME:-}"
				fi
			done
			sudo apt-get remove --purge --yes "${ARTIFACT}"
			echo "INFO: apt purged ${ARTIFACT}"
			if [[ -f "${ZITI_ENV_FILE:-}" ]]
			then
				sudo rm -fv "${ZITI_ENV_FILE}"
			fi
			if [[ -f "${ZITI_CRED_FILE:-}" ]]
			then
				sudo rm -fv "${ZITI_CRED_FILE}"
			fi
		else
			echo "INFO: not removing ${ARTIFACT} because it is a dependency of the service packages"
		fi
	}

	if [[ ${INSTALL} == true && ${ARCH} == amd64 ]]
	then
		sudo apt-get install --reinstall --yes --allow-downgrades "${TMPDIR}/${ARTIFACT}_${ZITI_VERSION#v}~${ZITI_REV}_${ARCH}.deb"
		echo "INFO: apt installed ${TMPDIR}/${ARTIFACT}_${ZITI_VERSION#v}~${ZITI_REV}_amd64.deb"

		if [[ ${ARTIFACT} == openziti ]]
		then
			BUILDSUM=$(sha256sum $ARTIFACTS_DIR/$ARCH/linux/ziti | awk '{print $1}')
			INSTALLSUM=$(sha256sum /opt/openziti/bin/ziti | awk '{print $1}')
			if [[ $BUILDSUM != "$INSTALLSUM" ]]
			then
				echo "Checksums do not match"
				exit 1
			fi
		fi
	fi
done

if [[ ${DOCKER} == true ]]
then
	# Ensure dependent docker images can reference a locally-built ziti-cli image.
	# If the requested artifacts include controller or router, make sure openziti is built first.
	needs_cli=false
	for ARTIFACT in "${ARTIFACTS[@]}"
	do
		if [[ ${ARTIFACT} == openziti-controller || ${ARTIFACT} == openziti-router ]]
		then
			needs_cli=true
			break
		fi
	done
	if [[ ${needs_cli} == true ]]
	then
		found_cli=false
		for ARTIFACT in "${ARTIFACTS[@]}"
		do
			if [[ ${ARTIFACT} == openziti ]]
			then
				found_cli=true
				break
			fi
		done
		if [[ ${found_cli} == false ]]
		then
			echo "INFO: Adding 'openziti' to artifacts to satisfy docker image dependency (controller/router require ziti-cli base image)" >&4
			ARTIFACTS=(openziti "${ARTIFACTS[@]}")
		fi
	fi

	# Build order: ziti-cli first (if present), then the rest.
	ARTIFACTS_ASC=()
	for ARTIFACT in "${ARTIFACTS[@]}"
	do
		if [[ ${ARTIFACT} != openziti ]]
		then
			ARTIFACTS_ASC+=("${ARTIFACT}")
		fi
	done
	if [[ ${needs_cli} == true ]]
	then
		ARTIFACTS_ASC=(openziti "${ARTIFACTS_ASC[@]}")
	fi
	echo "DEBUG: ARTIFACTS_ASC=${ARTIFACTS_ASC[*]}" >&3

	# Build tag used for the locally-built cli image.
	LOCAL_ZITI_CLI_IMAGE="ziti-cli"
	LOCAL_ZITI_CLI_TAG="${ZITI_VERSION#v}-${ZITI_REV}"
	for ARTIFACT in "${ARTIFACTS_ASC[@]}"
	do
		setArtifactVars "$ARTIFACT"
		for ARCH in "${ARCHS[@]}"
		do
			# Controller/router Dockerfiles use a ziti-cli base image. Always reference the local
			# image (built earlier in this loop) to avoid failing on missing registry tags.
			ZITI_CLI_IMAGE_BUILD_ARG="${HUB_USER}/ziti-cli"
			ZITI_CLI_TAG_BUILD_ARG="${ZITI_VERSION#v}-${ZITI_REV}"
			if [[ ${ARTIFACT} == openziti-controller || ${ARTIFACT} == openziti-router ]]
			then
				ZITI_CLI_IMAGE_BUILD_ARG="${LOCAL_ZITI_CLI_IMAGE}"
				ZITI_CLI_TAG_BUILD_ARG="${LOCAL_ZITI_CLI_TAG}"
			fi

			# ziti-cli Dockerfile expects the ziti-builder image to exist locally. Build it first,
			# and ensure it is used during the build.
			if [[ ${ARTIFACT} == openziti ]]
			then
				buildZitiGoBuilder
				runZitiGoBuilder "$ARCH"
			fi

			docker buildx build \
				--build-arg ZITI_CLI_IMAGE="${ZITI_CLI_IMAGE_BUILD_ARG}" \
				--build-arg ZITI_CLI_TAG="${ZITI_CLI_TAG_BUILD_ARG}" \
				--build-arg DOCKER_BUILD_DIR="./dist/docker-images/${ARTIFACT_SHORT}" \
				--platform="linux/${ARCH}" \
				--tag "${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}" \
				--tag "${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}" \
				--file "./dist/docker-images/${ARTIFACT_SHORT}/Dockerfile" \
				--load \
				"$PWD" 2>&3
			echo "INFO: Built Docker image ${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}"

			# Ensure the local cli tag exists for dependent builds, even if HUB_USER differs.
			if [[ ${ARTIFACT} == openziti ]]
			then
				docker tag "${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}" "${LOCAL_ZITI_CLI_IMAGE}:${LOCAL_ZITI_CLI_TAG}" 2>&3 || true
			fi

			if [[ ${PUSH} == true ]]
			then
				# Only push artifacts that were explicitly requested (not auto-added dependencies).
				explicit=false
				for EXPLICIT_ARTIFACT in "${EXPLICIT_ARTIFACTS[@]}"
				do
					if [[ "${EXPLICIT_ARTIFACT}" == "${ARTIFACT}" ]]
					then
						explicit=true
						break
					fi
				done
				if [[ ${explicit} == true ]]
				then
					docker push "docker.io/${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}"
					echo "INFO: Pushed Docker image ${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}"
				else
					echo "INFO: Skipping push for auto-added artifact '${ARTIFACT}'" >&4
				fi
			fi
		done
	done
fi
