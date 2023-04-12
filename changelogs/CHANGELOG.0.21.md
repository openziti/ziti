# Release 0.21.0

## Semantic now Required for policies (BREAKING CHANGE)

Previouxly semantic was optional when creating or updating policies (POST or PUT), defaulting to `AllOf` when not
specified. It is now required.

## What's New

* Bug fix: Using PUT for policies without including the semantic would cause them to be evaluated using the AllOf
  semantic
* Bug fix: Additional concurrency fix in posture data
* Feature: Ziti CLI now supports a comprehensive set of `ca` and `cas` options
* Feature: `ziti ps` now supports `set-channel-log-level` and `clear-channel-log-level` operations
* Change: Previouxly semantic was optional when creating or updating policies (POST or PUT), defaulting to `AllOf` when
  not specified. It is now required.
