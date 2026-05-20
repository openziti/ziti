#!/usr/bin/env bash
#
# Builds and pushes the openziti/quickstart multi-arch Docker image for a given
# release tag. Idempotent:
#
#   - skips the :vX.Y.Z push if that tag already exists in the registry
#   - moves :latest only when this tag IS the GitHub "Latest release" (or when
#     --force-latest is passed), AND :latest does not already point at the same
#     digest as :vX.Y.Z
#
# Designed to be safely re-runnable from CI or a developer laptop.
#
# Usage:
#   release-quickstart-image.sh --tag vX.Y.Z [--image-repo R] [--force-latest] [--dry-run]
#
# Required environment:
#   - docker CLI with buildx + an active builder (workflow does this)
#   - logged in to the target registry (workflow does this)
#   - GITHUB_TOKEN exported (used by `gh` to query the "Latest release" flag)
#   - GITHUB_REPOSITORY  (e.g. openziti/ziti) -- set by GitHub Actions; pass
#     explicitly when running locally
#

set -o errexit
set -o nounset
set -o pipefail

TAG=""
IMAGE_REPO="${ZITI_QUICKSTART_IMAGE:-docker.io/openziti/quickstart}"
FORCE_LATEST="false"
DRY_RUN="false"
CONTEXT_DIR=""

usage() {
    cat <<EOF
Usage: $0 --tag vX.Y.Z [options]

Options:
  --tag vX.Y.Z          Release tag to build the image for (required).
  --image-repo R        Image repo (default: \$ZITI_QUICKSTART_IMAGE or docker.io/openziti/quickstart).
  --force-latest        Move :latest to this tag even if GitHub does not mark this release as latest.
  --dry-run             Print actions but do not build or push.
  --context-dir DIR     Path to the Docker build context (default: repo-relative quickstart/docker/image).
  -h, --help            Show this help.
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --tag)           TAG="$2"; shift 2 ;;
        --image-repo)    IMAGE_REPO="$2"; shift 2 ;;
        --force-latest)  FORCE_LATEST="true"; shift ;;
        --dry-run)       DRY_RUN="true"; shift ;;
        --context-dir)   CONTEXT_DIR="$2"; shift 2 ;;
        -h|--help)       usage; exit 0 ;;
        *) echo "ERROR: unknown arg '$1'" >&2; usage >&2; exit 2 ;;
    esac
done

if [[ -z "$TAG" ]]; then
    echo "ERROR: --tag is required" >&2
    usage >&2
    exit 2
fi

if ! [[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: --tag '$TAG' is not a release semver (expected vMAJOR.MINOR.PATCH)" >&2
    exit 2
fi

if [[ -z "$CONTEXT_DIR" ]]; then
    # default: assume we were invoked from a checkout root
    CONTEXT_DIR="quickstart/docker/image"
fi

if [[ ! -d "$CONTEXT_DIR" ]]; then
    echo "ERROR: build context dir not found: $CONTEXT_DIR" >&2
    exit 2
fi

VERSION_NO_V="${TAG#v}"
TAGGED_REF="${IMAGE_REPO}:${VERSION_NO_V}"
LATEST_REF="${IMAGE_REPO}:latest"

run() {
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "[dry-run] $*"
    else
        echo "+ $*"
        eval "$@"
    fi
}

image_exists() {
    # returns 0 if the image ref resolves in the registry
    docker buildx imagetools inspect "$1" >/dev/null 2>&1
}

image_digest() {
    # prints the manifest list digest for a ref, or empty if it does not exist
    docker buildx imagetools inspect "$1" --format '{{.Manifest.Digest}}' 2>/dev/null || true
}

echo ""
echo "============================================================"
echo "  Release quickstart image"
echo "============================================================"
echo "  Tag (input):     $TAG"
echo "  Image repo:      $IMAGE_REPO"
echo "  Tagged ref:      $TAGGED_REF"
echo "  Latest ref:      $LATEST_REF"
echo "  Force :latest:   $FORCE_LATEST"
echo "  Build context:   $CONTEXT_DIR"
echo "  Dry run:         $DRY_RUN"
echo "============================================================"
echo ""

# ---------------------------------------------------------------- :vX.Y.Z push
echo ""
echo "---- Step 1: build & push $TAGGED_REF -----------------------"
echo ""

if image_exists "$TAGGED_REF"; then
    echo "INFO: $TAGGED_REF already exists in the registry; skipping build & push."
else
    echo "INFO: $TAGGED_REF not found in the registry; building and pushing."
    run "docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --build-arg ZITI_VERSION_OVERRIDE=${TAG} \
        --build-arg GITHUB_REPO_OWNER=${GITHUB_REPOSITORY%%/*} \
        --build-arg GITHUB_REPO_NAME=${GITHUB_REPOSITORY##*/} \
        --tag ${TAGGED_REF} \
        --push \
        ${CONTEXT_DIR}"
fi

# --------------------------------------------------------- :latest evaluation
echo ""
echo "---- Step 2: evaluate :latest promotion ---------------------"
echo ""

PROMOTE_LATEST="false"

if [[ "$FORCE_LATEST" == "true" ]]; then
    echo "INFO: --force-latest set; will move :latest to ${TAG}."
    PROMOTE_LATEST="true"
else
    if [[ -z "${GITHUB_REPOSITORY:-}" ]]; then
        echo "ERROR: GITHUB_REPOSITORY is not set; cannot query GitHub for the latest release." >&2
        echo "       Set it (e.g. GITHUB_REPOSITORY=openziti/ziti) or pass --force-latest." >&2
        exit 2
    fi

    LATEST_RELEASE_TAG="$(gh release view --repo "${GITHUB_REPOSITORY}" --json tagName --jq '.tagName' 2>/dev/null || true)"
    if [[ -z "$LATEST_RELEASE_TAG" ]]; then
        echo "WARN: could not determine GitHub's 'Latest release' tag; skipping :latest promotion."
    elif [[ "$LATEST_RELEASE_TAG" == "$TAG" ]]; then
        echo "INFO: GitHub marks ${TAG} as the latest release; will move :latest."
        PROMOTE_LATEST="true"
    else
        echo "INFO: GitHub's latest release is ${LATEST_RELEASE_TAG}, not ${TAG}; will NOT move :latest."
    fi
fi

# ---------------------------------------------------------- :latest retag
echo ""
echo "---- Step 3: move :latest if needed -------------------------"
echo ""

if [[ "$PROMOTE_LATEST" != "true" ]]; then
    echo "INFO: :latest promotion not requested; nothing to do."
else
    TAGGED_DIGEST="$(image_digest "$TAGGED_REF")"
    LATEST_DIGEST="$(image_digest "$LATEST_REF")"

    if [[ -z "$TAGGED_DIGEST" ]]; then
        if [[ "$DRY_RUN" == "true" ]]; then
            echo "[dry-run] would have built $TAGGED_REF in step 1; skipping :latest digest compare."
        else
            echo "ERROR: $TAGGED_REF has no digest after the build; aborting :latest move." >&2
            exit 1
        fi
    elif [[ "$TAGGED_DIGEST" == "$LATEST_DIGEST" ]]; then
        echo "INFO: :latest already points at the same digest as ${TAG} (${TAGGED_DIGEST}); skipping."
    else
        echo "INFO: moving :latest from '${LATEST_DIGEST:-<none>}' to '${TAGGED_DIGEST}'."
        run "docker buildx imagetools create --tag ${LATEST_REF} ${TAGGED_REF}"
    fi
fi

echo ""
echo "============================================================"
echo "  Done."
echo "============================================================"
echo ""
