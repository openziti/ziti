# Release 2.1.0

## What's New

* Config Type Target Field

## Config Type Target Field

Config types now have an optional `target` field that indicates what kind of entity the config type
is intended for. Valid values are `"service"` and `"router"`. The field is set on creation and is
immutable afterward.

This is the first step toward controller-managed router configuration. The `target` field lets us
distinguish between config types meant for services and config types meant for routers, which keeps
UIs, APIs, and validation clean. See `doc/design/ctrl-managed-router-config.md` for the full design.

A database migration sets `target = "service"` on all existing config types. Services and identity
service config overrides now require that referenced configs have a config type with
`target = "service"`.

The CLI has been updated to support the new field:

* `ziti edge create config-type` now accepts a `--target` flag
* `ziti edge list config-types` now shows a `Target` column
