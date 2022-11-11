# Releasing Ziti

Release happen by merging from the release-next branch to main.

## Release pre-requisites

1. Make sure you have a clean build in GitHub Actions.
2. Make sure you have a clean build in Jenkins smoketest (until smoketest is superceded by fablab smoke in GH)
3. Make sure CHANGELOG.md is up to date
    1. You can use `ziti-ci build-release-notes` to generate ziti library version updates and issues fixed,
       as long as the git commit has `fixed #<issue number>` (or fixes, closes, closed, etc)
4. Create a PR to merge release-next to main, once it's approved you can merge and the release will happen.

