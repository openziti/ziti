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

2. Ensure the `go build` command succeeds. It' not necessary to store the build artifact(s).

    ```bash
    go build ./...
    ```

3. Ensure the `go test` command succeeds.

    ```bash
    go test ./...
    ```

4. Ensure PR checks succeed.
    1. Make sure you have a clean build in GitHub Actions.
    2. Make sure you have a clean build in fablab smoketest.
5. Ensure CHANGELOG.md is up to date.
    1. Run `ziti-ci build-release-notes` in your PR branch to generate library version updates and summarize issues
    fixed, as long as the git commit has `fixed #<issue number>` (or fixes, closes, closed, etc.).
    1. Sanity-check and paste the output into CHANGELOG.md under a heading like `## Component Updates and Bug Fixes`.

## Release Pre-requisites

Perform these steps in the release-next (trunk) branch which is based on main to release Ziti.

1. Create a PR to merge release-next to main. Release happens by merging from the release-next branch to main.
2. Ensure PR checks succeed.
