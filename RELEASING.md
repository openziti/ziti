# Releasing Ziti

## Pre-requisites to Merge to Default Branch

Perform these steps in PR branches based on **main**. This is the default branch and represents a revision that is
a candidate for release.

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

## Pre-Release

Perform these steps on **main** (the default branch) to create a binary pre-release.

1. Ensure checks succeed on the default branch. Downstreams will not be released if any checks fail on same revision where a release is created.
1. Push a tag like v*, typically on default branch HEAD to trigger the pre-release workflow named `release.yml`.

## Stable and Latest Release

Pre-releases are releases, but they're not promoted as "latest" in GitHub or automatically shipped downstream. Marking a
release as not a prerelease makes it a stable release. There can be one stable release that's also marked "latest"
(`isLatest: true`).

1. After an arbitrary burn-in period, unmark "prerelease" in GitHub Releases (`isPrerelease: false`). This will automatically promote and advertise the downstreams.
   Note: the downstreams workflow trigger ignores `isLatest`, can only be triggered once for a release, and waits for all other checks on the same revision.

## Downstreams

These downstreams are built on push to the default branch **main** and release tags.

- Linux packages
  - `openziti` - provides `/usr/bin/ziti`
  - `openziti-controller` - provides `ziti-controller.service`
  - `openziti-router` - provides `ziti-router.service`
- Container Images
  - `openziti/ziti-cli` - provides `/usr/local/bin/ziti`
  - `openziti/ziti-controller` - built from `ziti-cli` (`/usr/local/bin/ziti`) and `ziti-console-assets` (`/ziti-console`) and executes `ziti controller run`
  - `openziti/ziti-router` - built from `ziti-cli`and executes `ziti router run`

### Promoting Downstreams

The downstream artifacts are named and handled as follows.

- push to **main**
  - Linux packages are published in the test repos with a release candidate semver, e.g. `1.0.1~123` where `1.0.0` is the highest semver tag in the repo and `123` is the build number. These release candidate semvers are higher versions than latest release.
  - Container images are pushed to the `:main` repo tag.
- push to release tag
  - Linux packages are published in the test repos with a release semver, e.g. `1.0.1`.
  - Container images are pushed to a release semver tag, e.g. `:1.0.1`.
- GitHub binary pre-release is marked "latest"
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

### Manually Promoting Downstreams

If downstream promotion failed for any reason, e.g., a check failure on the same Git revision blocked promotion, then it
is best to create a new release that fixes the problem. Manually promoting downstreams is hypothetically possible, has
never been attempted, is error prone and tedious, and should probably be avoided.

## Quickstart Releases

See [the quickstart release README](quickstart/README.md).
