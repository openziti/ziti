# Quickstart

The Ziti quickstart documentation is here: [Ziti Network Quickstarts](https://openziti.io/docs/category/network).

## Releasing a new version of the Quickstart

### Artifacts Produced by a Release

The enclosing project's GitHub releases are never updated and no Git tags are created for a quickstart release.

1. `openziti/quickstart` container image [in Docker Hub](https://hub.docker.com/r/openziti/quickstart)
1. a CloudFront Function in AWS pointing the `get.openziti.io` reverse proxy to the GitHub SHA of the release

### Release Process

A quickstart release is triggered automatically by GitHub release events of the enclosing project:

- `release: published` builds and pushes `openziti/quickstart:X.Y.Z` for a stable release tag `vX.Y.Z`
  (prerelease tags such as `v2.1.0-pre1` are skipped).
- `release: edited` (e.g., a release is marked "Latest release") re-points `:latest` without rebuilding the image.

GitHub runs the copy of the workflow at the release tag's commit, so each release branch must carry this workflow.

The `:latest` tag and the `get.openziti.io` CloudFront Function only follow GitHub's "Latest release". Maintenance
releases of older minor versions publish their versioned image but leave `:latest` and CloudFront alone.

The release-quickstart workflow can also be invoked manually from the GitHub Actions UI
(`workflow_dispatch`) with an explicit release tag (e.g., `v1.6.7`). Use this when:

- the automatic run failed for transient reasons and you want to rerun it, or
- you need to republish the quickstart artifacts for an existing release without cutting a new one.

Pass `force_latest: true` or `force_cloudfront: true` to move `:latest` or deploy CloudFront for a release that is
not GitHub's "Latest release".

Both jobs in the workflow are **idempotent and independently re-runnable**:

- The Docker image push step checks the registry first and skips the build if `openziti/quickstart:X.Y.Z`
  already exists. `:latest` is only moved if it doesn't already point at the same digest.
- The CloudFront deploy is a re-publish each time it runs; AWS handles repeated identical configurations
  cleanly.

If one job succeeds and the other fails, you can rerun the failed job alone from the Actions UI without
redoing the work that already succeeded.

### Release Machinery

The release process is encoded in [a GitHub workflow](../.github/workflows/release-quickstart.yml).
The bulk of the Docker logic lives in [`dist/scripts/release-quickstart-image.sh`](../dist/scripts/release-quickstart-image.sh),
which is runnable locally for testing.

### GitHub Raw Reverse Proxy

The `get.openziti.io` reverse proxy is a CloudFront distribution that points to a CloudFront Function and serves as a
shorter HTTP URL getter for raw GitHub source files, e.g. `https://get.openziti.io/dock/simplified-docker-compose.yml`.
The CloudFront Function is a JavaScript function that looks at the URI path of the incoming request and forwards it to
the appropriate GitHub raw download path. The CloudFront Function is updated by the release process, and the CloudFront
Distribution itself is hand-maintained in the AWS Console. The Distribution has these characteristics:

* Viewer Domain Name: `get.openziti.io` (frontend)
* Route Origin: `raw.githubusercontent.com` (backend, upstream)
* Auto-renewing TLS certificate from ACM
* Cache Policy `CachingOptimized` (default)
* Routes to Origin based on Javascript Function deployed by quickstart release

You can add or change a GitHub raw shortcut route by modifying the [routes.yml](../dist/cloudfront/get.openziti.io/routes.yml) file.
