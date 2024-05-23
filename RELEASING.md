# Releasing Ziti

## Release-next Pre-requisites

Perform these steps in PR branches based on release-next (trunk).

1. Tidy dependencies.
    1. Ensure you have downloaded the `@latest` artifact from the dependency(ies) you are updating in the main Ziti project, e.g.,

        ```bash
        go get -u github.com/openziti/edge@latest
        ```

    2. Run `go mod tidy` in the main Ziti project and in the `./zititest` sub-tree.

        ```bash
        go mod tidy
        cd ./zititest
        go mod tidy
        cd ..
        ```

2. Ensure the `go test` command succeeds. This will also ensure the project builds.

    ```bash
    go test ./...
    ```

3. Ensure PR checks succeed.
    1. Make sure you have a clean build in GitHub Actions.
    2. Make sure you have a clean build in fablab smoketest.
4. Ensure CHANGELOG.md is up to date.
    1. Run `ziti-ci build-release-notes` in your PR branch to generate library version updates and summarize issues. Note that you will need a working copy of each module that has changed in an adjacent directory with the default repo name in order for this to work.
    fixed, as long as the git commit has `fixed #<issue number>` (or fixes, closes, closed, etc.).
    1. Sanity-check and paste the output into CHANGELOG.md under a heading like `## Component Updates and Bug Fixes`.

### Shell Script to Tidy Dependencies

```bash
(
  set -euxo pipefail
  go list -m -f '{{ .Path }} {{ .Main }}' all \
    | grep ziti | grep -v "$(go list -m)" | grep -v dilithium | cut -f 1 -d ' ' \
    | xargs -n1 /bin/bash -c 'echo "Checking for updates to $@";go get -u -v $@;' ''
  go mod tidy
  if git diff --quiet go.mod go.sum; then
    echo "no changes"
  else
    echo "dependency updates found"
  fi

  if [ -f "zititest/go.mod" ]; then
    echo "./zititest$ go mod tidy"
    cd zititest
    go mod tidy
    cd ..
  fi
  ziti-ci build-release-notes
)
```

## Release Pre-requisites

Perform these steps in the release-next (trunk) branch which is based on main to release Ziti.

1. Create a PR to merge release-next to main. Release happens by merging from the release-next branch to main.
2. Ensure PR checks succeed.

## Downstreams

These downstreams are built on push to GitHub branches: **release-next**, **release-v***, and **main**.

- Linux packages
  - `openziti` - provides `/usr/bin/ziti`
  - `openziti-controller` - provides `ziti-controller.service`
  - `openziti-router` - provides `ziti-router.service`
- Container Images
  - `openziti/ziti-cli` - provides `/usr/local/bin/ziti`
  - `openziti/ziti-controller` - built from `ziti-cli` and `ziti-console-assets` and executes `ziti controller run`
  - `openziti/ziti-router` - built from `ziti-cli`and executes `ziti router run`

### Promoting Downstreams

The downstream artifacts are named and handled as follows.

- push to `release-next`
  - Linux packages are published in the test repos with a release candidate semver, e.g. `1.0.1~123` where `1.0.0` is the highest semver tag in the repo and `123` is the build number. These release candidate semvers are higher versions than latest so that they are installed by default on devices subscribed to a test repo.
  - Container images are pushed to the `:release-next` tag.
- push to `release-v*` or `main`
  - Linux packages are published in the test repos with a release semver, e.g. `1.0.1`.
  - Container images are pushed to a release semver tag, e.g. `:1.0.1`.
- GitHub pre-release is marked "latest"
  - Linux packages for the release are copied from the "test" repos to the "stable" repos.
  - Container images' semver release tags are re-tagged as `:latest`.

### Rolling Back Downstreams

If a release is found to be faulty, the downstream artifacts can be rolled back as follows.

The first step is to ensure the GitHub release is not marked "latest," and the highest good release is marked "latest." Do not delete the faulty release (assets) or Git tag.

- Linux packages - The released semver is removed from the stable repo and must not be re-used. To arm this script, uncomment the `DELETE="--quiet"` line and set `BAD_VERSION`.

    ```bash
    (set -euxopipefail

      ARTIFACTORY_REPO='zitipax-openziti-(rpm|deb)-stable'
      DELETE="--dry-run"
      : DELETE="--quiet"
      BAD_VERSION=0.0.1

      declare -a ARTIFACTS=(openziti{,-controller,-router})

      if [[ $DELETE =~ quiet ]] && {
        echo "WARNING: permanently deleting" >&2;
        sleep 9;
      }

      for META in rpm.metadata deb;
      do
        for ARTIFACT in ${ARTIFACTS[@]};
        do
          while read;
          do
            jf rt search --props "${META}.name=${ARTIFACT};${META}.version=${BAD_VERSION}" "${REPLY}/*" \
            | jq '.[].path' \
            | xargs -rl jf rt del $DELETE;
          done< <(
            jf rt cl -sS /api/repositories \
            | jq --raw-output --arg artifactory "${ARTIFACTORY_REPO}" '.[]|select(.key|match($artifactory))|.key'
          )
        done
      done
    )
    ```

- Container images - The `:latest` tag is moved to the last good release semver. To ready the script, set `GOOD_VERSION`.

    ```bash
    (set -euxopipefail
      GOOD_VERSION=1.0.0

      for REPO in ziti-{cli,controller,router,tunnel}; do
          docker buildx imagetools create --tag openziti/${REPO}:latest openziti/${REPO}:${GOOD_VERSION}
      done
    )
    ```
