# Quickstart

The Ziti quickstart documentation is here: [Ziti Network Quickstarts](https://openziti.io/docs/category/network).

## Releasing a new version of the Quickstart

### Artifacts Produced by a Release

The enclosing project's GitHub releases are never updated and no Git tags are created for a quickstart release.

1. `openziti/quickstart` container image [in Docker Hub](https://hub.docker.com/r/openziti/quickstart)
1. a CloudFront Function in AWS pointing the `get.openziti.io` reverse proxy to the GitHub SHA of the release

### Release Process

A quickstart release is created when either of the following conditions are met:

1. OpenZiti, the enclosing project, is released by the OpenZiti team
1. A pull request is merged into the trunk branch `main` with the label `quickstartrelease`

### Release Machinery

The release process is encoded in [a GitHub workflow](../.github/workflows/release-quickstart.yml).

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
