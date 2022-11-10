
# Build

Please refer to [the local development README](./doc/002-local-dev.md) for build instructions.

## Crossbuilds

When you push to your repo fork then GitHub Actions will automatically crossbuild for several OSs and CPU architectures. You'll then be able to download the built artifacts from the GitHub UI. The easiest way to crossbuild the Linux exectuables locally is to build and run the crossbuild container. Please refer to [the crossbuild container README](../Dockerfile.linux-build.README) for those steps. For hints on crossbuilding for MacOS and Windows see [the main GitHub Actions workflow](../.github/workflows/main.yml) which defines the steps that are run when you push to GitHub.
