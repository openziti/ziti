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

Fetch updates for all non-main modules whose path contains 'ziti'.

```bash
(
  set -euxo pipefail
  go list -m -f '{{.Main}} {{.Path}}' all \
    | awk '$1 == "false" && $2 ~ /ziti/ {print $2}' \
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
   Note: the wait-for-all-checks gate was removed on **main**, so promotion there is gated only on the tag being a stable semver. Release branches may still carry the old gate. See [Re-running Promotion](#re-running-promotion) for why the branch matters.

## Hotfixes

A hotfix is released from a prior release, so it has a lower version than the highest release version. A hotfix can be marked stable (not a prerelease) like any other version, but is handled differently during stable promotion because it is not the highest release: Docker images are not tagged `:latest`, avoiding clobbering of the highest version. That is, the highest version should remain tagged `:latest` (the default image for consumers), not the newer hotfix.

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

### Re-running Promotion

The `released` event fires only once per release, so re-marking it stable will not promote again. Re-run `promote-downstreams.yml` with `workflow_dispatch` and the optional `tag` input instead. Promotion is idempotent: `jf rt copy` copies rather than moves, and re-tagging `:latest` with the same digest is a no-op.

```bash
gh workflow run promote-downstreams.yml --repo openziti/ziti --ref main -f tag=v2.0.1
```

The `--ref` matters. A `release` event runs the copy of the workflow that exists **on the release tag**, not the copy on the default branch, so a workflow fix merged to **main** does not apply to a release cut from a release branch that lacks it. Cherry-pick workflow fixes to the release branches, and dispatch with `--ref main` to promote using main's copy.

The copy jobs use `--fail-no-op=true`, so promotion fails when a source file is missing or misnamed in the test repos. Note the naming asymmetry: `openziti` is per-arch, while the `openziti-controller` and `openziti-router` meta packages are `_all.deb` and `.noarch.rpm` under each arch directory. A 404 there means publishing, not promotion, is what needs fixing.

### Verifying Promotion

A green promotion run does not prove consumers can install the release. Files land in the stable pool immediately, but Artifactory rebuilds the APT and YUM indexes asynchronously afterward, and until it does the package is reachable by direct URL yet invisible to `apt` and `dnf`.

```bash
(set -euxo pipefail
V=2.0.1

# is it in the stable index that clients actually read? (distribution 'debian', component 'main')
curl -s https://packages.openziti.org/zitipax-openziti-deb-stable/dists/debian/main/binary-amd64/Packages | grep -A1 '^Package: openziti$' | grep -F "${V}"

# when was that index last rebuilt?
curl -s https://packages.openziti.org/zitipax-openziti-deb-stable/dists/debian/Release | grep -i '^Date:'
)
```

On a client, `apt install --only-upgrade openziti` reporting "already the newest version" is the expected symptom of a stale local cache, not of a failed promotion. Run `sudo apt update` first.

### Release Notifications Never Block a Release

The Mattermost notification workflows (`mattermost-channel-posts.yml`, `mattermost-webhook.yml`) post over a ziti overlay service that has been unreachable for long stretches, and neither action sets a client-side connect timeout. Both therefore set `continue-on-error: true` and `timeout-minutes`. Leave those in place, and never let a chat notification job gate a release: a failed chat post has already blocked a stable promotion end to end, because the notification jobs were unintentionally in scope for a wait-for-all-checks gate.

### Rolling Back Downstreams

If a release is found to be faulty, the downstream artifacts can be rolled back as follows.

The first step is to ensure the GitHub release is not marked "latest," and the highest good release is marked "latest." Do not delete the faulty release (assets) or Git tag.

- Linux packages - Run [zititest/scripts/housekeeper-artifactory-zitipax.bash --help](./zititest/scripts/housekeeper-artifactory-zitipax.bash) for usage hints. The goal is to delete the bad semver from all Linux package repositories (all platforms, all package managers, etc.).

    Once the bad semver is removed from the stable repo, it must not be reused.

    You must target one or more artifact names, e.g., `--artifacts openziti openziti-console`.

    ```bash
    # dry run without confirmation prompts in all stable repos
    ./housekeeper-artifactory-zitipax.bash --stages release --artifacts openziti --version 2.3.4 --dry-run --quiet
    
    # destructive run with confirmation prompts in all stable repos
    ./housekeeper-artifactory-zitipax.bash --stages release --artifacts openziti --version 2.3.4
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
is probably best to create a new release that fixes the problem. Manually promoting downstreams is possible, but error
prone and tedious.

The first step is to identify the version that *should* be available in the downstream repos. In GitHub, find [the latest stable release](https://github.com/openziti/ziti/releases/latest). This is the highest version that's not a pre-release, and should be available in the downstream repos, i.e., Linux packages, Docker images, etc.

#### Manually Promoting Linux Packages

In Artifactory, explore the available non-tunneler packages. They're organized together because they are OS
version-neutral, while the tunneler packages are organized separately by OS version. DEB and RPM repos have distinct
layouts, but these links alone can answer "Is the latest stable CLI available?" by identifying the highest version of the `openziti` package, e.g., `openziti_1.5.4_amd64.deb`.

- [the `debian` tree](https://packages.openziti.org/zitipax-openziti-deb-stable/pool/openziti/amd64/)
- [the `redhat` tree](https://packages.openziti.org/zitipax-openziti-rpm-stable/redhat/x86_64/)

##### Manually Promoting RedHat Packages

Modify this example script to suit your needs.

```bash
(set -euxo pipefail
# curl -sSf https://api.github.com/repos/openziti/ziti/releases/latest | jq -r '.tag_name'
V=1.5.4
test -n "${V}"
for A in x86_64 aarch64 armv7hl; do
  for P in openziti{,-controller,-router}; do
    # openziti is built per arch; the controller and router meta packages are noarch
    case "${P}" in openziti) RPM_ARCH="${A}" ;; *) RPM_ARCH=noarch ;; esac
    jf rt cp \
      --recursive=false \
      --flat=true \
      --fail-no-op=true \
      zitipax-openziti-rpm-{test,stable}/redhat/${A}/${P}-${V}-1.${RPM_ARCH}.rpm
  done
done
)
```

##### Manually Promoting Debian Packages

Modify this example script to suit your needs.

```bash
(set -euxo pipefail
# V=$(curl -sSf https://api.github.com/repos/openziti/ziti/releases/latest | jq -r '.tag_name')
V=1.5.4
test -n "${V}"

for A in amd64 arm64 armhf; do
  for P in openziti{,-controller,-router}; do
    # openziti is built per arch; the controller and router meta packages are arch-independent
    case "${P}" in openziti) DEB_ARCH="${A}" ;; *) DEB_ARCH=all ;; esac
    jf rt cp \
      --recursive=false \
      --flat=true \
      --fail-no-op=true \
      zitipax-openziti-deb-{test,stable}/pool/${P}/${A}/${P}_${V}_${DEB_ARCH}.deb
  done
done
)
```

The `openziti-console` package is controlled separately by that project's release process in [openziti/ziti-console](https://github.com/openziti/ziti-console/blob/app-ziti-console-v3.12.3/.github/workflows/linux-publish.yml#L143).

#### Manually Promoting Docker Images

Docker images are routinely "promoted" by re-tagging the release tag, e.g., `:1.5.4`, as `:latest`.

Modify this example script to suit your needs.

```bash
(set -euxo pipefail
# V=$(curl -sSf https://api.github.com/repos/openziti/ziti/releases/latest | jq -r '.tag_name')
V=1.5.4
test -n "${V}"
for R in ziti-{cli,controller,router,tunnel}; do
  docker buildx imagetools create --tag openziti/${R}:latest openziti/${R}:${V}
done
)
```

Note: The `openziti/ziti-console-assets` image is controlled separately by the workflow in [openziti/ziti-console](https://github.com/openziti/ziti-console/blob/app-ziti-console-v3.12.3/.github/workflows/docker-publish.yml#L48).

## Quickstart Releases

See [the quickstart release README](./quickstart/README.md).
