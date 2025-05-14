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
			echo -e "Usage: $(basename $0) FLAGS"\
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

ARTIFACTS_DIR=./release
# export to nfpm and assign right after building ziti binary
export ZITI_VERSION ZITI_REV
: ${HUB_USER:=kbinghamnetfoundry}

cd ~/Sites/netfoundry/github/ziti

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

# sort to ensure the cli package is built last in case clean is true which must first be run on the services that depend
# on the CLI
mapfile -t ARTIFACTS_DESC < <(
	for i in "${ARTIFACTS[@]}"
	do
		printf '%d %s\n' ${#i} "$i"
	done \
	| sort -unr \
	| cut -d' ' -f2
)
echo "DEBUG: ARTIFACTS_DESC=${ARTIFACTS_DESC[*]}" >&3
for ARTIFACT in "${ARTIFACTS_DESC[@]}"
do

	setArtifactVars "$ARTIFACT"
	for ARCH in "${ARCHS[@]}"; do
		if [[ ${ARTIFACT} == openziti ]]; then
			# build the builder
			docker buildx build \
				--tag=ziti-go-builder \
				--build-arg uid="$UID" \
				--load \
				./dist/docker-images/cross-build/ 2>&3
			echo "INFO: Built ziti-go-builder"
			docker run \
				--rm \
				--user "$UID" \
				--name=ziti-go-builder \
				--volume="$PWD:/mnt" \
				--volume="${GOCACHE:-${HOME}/.cache/go-build}:/.cache/go-build" \
				--env GOEXPERIMENT="${GOEXPERIMENT:-}" \
				--env=GOCACHE=/.cache/go-build \
				ziti-go-builder $ARCH
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

		for PKG in deb rpm; do
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
		if [[ "${ARTIFACT}" =~ openziti-(controller|router) ]]; then
			for SVC in ${ARTIFACT_SHORT}.service; do
				#if systemctl list-unit-files | grep -qF "${SVC}"; then
				(set +e
					sudo systemctl stop "${SVC}"
					sudo systemctl disable --now "${SVC}"
					sudo systemctl reset-failed "${SVC}"
					sudo systemctl clean --what=state "${SVC}"
				)||true
				#fi
				echo "INFO: Cleaned ${SVC}"
				if [[ -n "$(sudo ls -A "${ZITI_HOME:-}" 2>/dev/null)" ]]
				then
					sudo rm -rf "${ZITI_HOME:-}/"
					echo "INFO: Cleaned non-empty ZITI_HOME=${ZITI_HOME:-}"
				fi
			done
			sudo apt-get remove --purge --yes "${ARTIFACT}"
			echo "INFO: apt purged ${ARTIFACT}"
			if [[ -f "${ZITI_ENV_FILE:-}" ]]; then
				sudo rm -fv "${ZITI_ENV_FILE}"
			fi
			if [[ -f "${ZITI_CRED_FILE:-}" ]]; then
				sudo rm -fv "${ZITI_CRED_FILE}"
			fi
		else
			echo "INFO: not removing ${ARTIFACT} because it is a dependency of the service packages"
		fi
	}

	if [[ ${INSTALL} == true && ${ARCH} == amd64 ]]; then
		sudo apt-get install --reinstall --yes --allow-downgrades "${TMPDIR}/${ARTIFACT}_${ZITI_VERSION#v}~${ZITI_REV}_${ARCH}.deb"
		# sudo apt-mark manual "${ARTIFACT}"
		echo "INFO: apt installed ${TMPDIR}/${ARTIFACT}_${ZITI_VERSION#v}~${ZITI_REV}_amd64.deb"

		if [[ ${ARTIFACT} == openziti ]]; then
			BUILDSUM=$(sha256sum $ARTIFACTS_DIR/$ARCH/linux/ziti | awk '{print $1}')
			INSTALLSUM=$(sha256sum /opt/openziti/bin/ziti | awk '{print $1}')
			if [[ $BUILDSUM != "$INSTALLSUM" ]]; then
				echo "Checksums do not match"
				exit 1
			fi
		fi
	fi
	
	# the first two build args are ignored by the ziti-cli Dockerfile and used only by controller and router builds to
	# source the CLI image
done

if [[ ${DOCKER} == true ]]
then
	# sort to ensure the cli image is built first so it can be source by the controller and router builds
	mapfile -t ARTIFACTS_ASC < <(
		for i in "${ARTIFACTS[@]}"
		do
			printf '%d %s\n' "${#i}" "$i"
		done \
		| sort -un \
		| cut -d' ' -f2
	)
	echo "DEBUG: ARTIFACTS_ASC=${ARTIFACTS_ASC[*]}" >&3
	for ARTIFACT in "${ARTIFACTS_ASC[@]}"
	do
		setArtifactVars "$ARTIFACT"
		for ARCH in "${ARCHS[@]}"
		do
			docker buildx build \
				--build-arg ZITI_CLI_IMAGE="${HUB_USER}/ziti-cli" \
				--build-arg ZITI_CLI_TAG="${ZITI_VERSION#v}-${ZITI_REV}" \
				--build-arg DOCKER_BUILD_DIR="./dist/docker-images/${ARTIFACT_SHORT}" \
				--platform="linux/${ARCH}" \
				--tag "${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}" \
				--tag "${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}" \
				--file "./dist/docker-images/${ARTIFACT_SHORT}/Dockerfile" \
				--load \
				"$PWD" 2>&3
			echo "INFO: Built Docker image ${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}"

			if [[ ${PUSH} == true ]]
			then
				docker push "docker.io/${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}"
				echo "INFO: Pushed Docker image ${HUB_USER}/${ARTIFACT_SHORT}:${ZITI_VERSION#v}-${ZITI_REV}"
			fi
		done
	done
fi
