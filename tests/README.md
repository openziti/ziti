# Writing tests in `tests/`

This is the guide to read before adding a test to this package. It describes how
new tests should be written.

These are **integration tests**. Each spins up an in-process controller (and, for
dataflow tests, routers) and exercises it mostly through its public REST APIs —
**black-box** testing. When a test needs to, it can also reach into the running
controller or router internals (e.g. hooking the event dispatcher) — **gray-box**
testing. Which one you use depends on what the test needs to observe.

The package has accumulated tests since 2019 and the
style has shifted: older tests (`service_test.go`, `identity_test.go`, `ca_test.go`)
drive everything through the legacy `AdminManagementSession` with untyped gabs JSON,
raw query strings, and `require*` helpers that assert internally. Newer tests
(roughly 2024 onward — `auth_oidc_csr_test.go`, `oidc_error_codes_test.go`,
`circuit_failure_cause_test.go`, `role_attribute_usage_test.go`) use the typed,
generated API clients, return errors from helpers so the test does the asserting,
explain themselves with comments and messages, and isolate their data. Follow the
newer style. The rest of this document spells out what that means.

## Running the tests

These tests are gated behind build tags, so a plain `go test ./...` skips them all.
Pass the tags for the suite you want. Run from the repo root.

| Tag         | Selects                                                        |
| ----------- | ------------------------------------------------------------- |
| `apitests`  | API integration tests (the bulk of this package)              |
| `dataflow`  | Tests that move traffic through in-process routers (slower)   |
| `perftests` | Performance tests (some also require `apitests`)              |
| `cli_tests` | CLI tests in the `cli_tests/` subdir (need a built `ziti`)    |

```bash
# all apitests in the package
go test -tags apitests ./tests/...

# dataflow tests (give them a longer timeout — they start routers)
go test -tags dataflow -timeout 15m ./tests/...

# both suites at once (space- or comma-separated tags both work)
go test -tags "apitests dataflow" -timeout 15m ./tests/...
```

**Workflow:** while developing, run just the new test or the test under work first
with `-run` for fast feedback — then **always run the entire suite** before
considering the change done, to catch interactions with shared state.

Run a specific test, or a `|`-separated set, with `-run`:

```bash
go test -tags apitests -run 'Test_RoleAttributeUsageEndpoints' ./tests/...
go test -tags apitests -run 'Test_Authenticate_Updb|Test_Lockout' ./tests/...
```

`-run` takes a regex matched against the test name, and subtest names are joined
with `/` (spaces become `_`). To target one nested subtest:

```bash
# Test_X -> t.Run("identity role-attribute usage") -> t.Run("filter narrows result set")
go test -tags apitests -run 'Test_X/identity_role-attribute_usage/filter_narrows_result_set' ./tests/...
```

CLI tests additionally need a built binary pointed at by an env var:

```bash
go build -o ./cli_tests_bin/ziti ./ziti
ZITI_CLI_TEST_ZITI_BIN="$PWD/cli_tests_bin/ziti" go test -tags cli_tests ./tests/cli_tests/...
```

## File basics

- **Build tag first.** `//go:build apitests` for API tests, `//go:build dataflow`
  for tests that move traffic through routers. Don't omit the tag; a few recent
  files did, and it lands them in the wrong test runs.
- **Apache license header** after the build tag, then `package tests`.
- **One feature area per file**, named after the thing under test
  (`auth_oidc_csr_test.go`, `circuit_failure_cause_test.go`).

## Test skeleton

Every test starts the same way:

```go
func Test_Thing(t *testing.T) {
    ctx := NewTestContext(t)
    defer ctx.Teardown()
    ctx.StartServer()
    // optional: only if you use the older ctx.AdminManagementSession.requireNew*
    // fixture helpers — this is what establishes ctx.AdminManagementSession
    ctx.RequireAdminManagementApiLogin()
    ...
}
```

Use subtests heavily and name them as behavior sentences — `"updb auth with CSR
returns session cert"`, `"filter narrows result set"`. The **first line of every
subtest must re-bind the testing context**:

```go
t.Run("withIds=true populates entity id arrays", func(t *testing.T) {
    ctx.testContextChanged(t)   // or the newer alias ctx.NextTest(t)
    ...
})
```

`ctx.Req` and friends are bound to a specific `*testing.T`. Forgetting this line
causes assertions to report against the wrong `*testing.T`.

### Nest dependent sub-tests

If one subtest depends on state another subtest creates (e.g. a list test that
needs a prior create), **nest its `t.Run()` inside the `t.Run()` it depends on** —
do not place dependent subtests as siblings. Sibling subtests must be independent
and runnable in any order. Nesting makes the dependency explicit and keeps the
created entities in scope:

```go
t.Run("create identity", func(t *testing.T) {
    ctx.testContextChanged(t)
    id := ctx.AdminManagementSession.requireNewIdentity(false, attr)

    t.Run("list includes the new identity", func(t *testing.T) {
        ctx.testContextChanged(t)
        // depends on the create above, so it lives inside it
    })
})
```

> Subtest bodies are closures, and that is expected — the "no inline closures" rule
> below does **not** apply to `t.Run` bodies. Use subtests freely.

## Arrange, Act, Assert

Structure each test and subtest in three phases, in order, separated by a blank
line (add a short comment when it helps):

- **Arrange** — build the *fixtures* the test needs (see [Fixtures and data
  isolation](#fixtures-and-data-isolation)).
- **Act** — perform the one operation under test.
- **Assert** — check the result.

Keep **Act** to the single thing the test is about; everything before it is Arrange,
everything after is Assert. Lean tests this way wherever the pattern fits.

## Closures and helpers

**Do not define inline function closures inside test functions.** A closure
assigned to a local variable and then called (or passed around) inside a test is
hard to read and hides control flow. This rule targets helper-style closures —
**not** `t.Run` subtest bodies, which are closures by design and are encouraged.

Tests are allowed to repeat themselves; **duplication is preferred over premature
abstraction.** A few extra lines spelled out in each subtest read better than a
clever helper that you have to jump to in order to understand the test.

### Two flavors of helper

Helpers come in two distinct flavors, and the rules differ for each:

1. **Fixture-setup helpers** build the world a test runs against — create and enroll
   an identity, authenticate via OIDC and return the token, build a client with a
   static bearer token. **A fixture-setup helper must never assert.** It returns
   `(value, error)` and the caller checks the error. This keeps the failure
   reported at the caller and keeps the helper reusable across tests with different
   failure expectations.

2. **Asserting helpers** encapsulate a repeated *assertion* (not setup). Because
   `ctx.Req` embeds testify's `require.Assertions`, testify is `t.Helper()`-aware
   and skips its own frames — but a hand-written wrapper that calls `ctx.Req.*` adds
   a frame testify cannot skip for you. **An asserting helper must therefore call
   `ctx.T().Helper()` (or `t.Helper()`) as its first line**, so a failure is
   reported at the caller, not inside the helper.

**Prefer fixture-setup helpers; avoid asserting helpers.** An asserting helper is
justified only when it *truly* removes several lines of assertion setup that would
otherwise be inlined in **more than three or four** places, and inlining it would
**dramatically** hurt readability. This is a rule of thumb, not a hard threshold; a
simple gauge is **(lines of assertion) × (number of call sites)** — a one-line
check repeated five times is not worth a helper, but a six-line check repeated
across many subtests may be. When in doubt, inline and let the test repeat itself.

### Where a helper lives

Once you've decided a helper is genuinely warranted, choose its home deliberately:

| What to factor out                             | Where it goes                                                                              |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Reusable REST op / fixture setup               | Method on `ManagementHelperClient` / `ClientHelperClient`; returns `(value, error)`; godoc |
| True context-level concern (no API call)       | Method on `TestContext` (context-level behavior only)                                      |
| Pure computation, API-free, used in >3 places  | Private package-level func at file bottom; godoc                                           |
| Justified asserting helper (see rule of thumb) | Private package-level func; `t.Helper()` first line; godoc                                 |
| Used once, or just builds an inline struct     | Inline it; don't write the helper                                                          |

A few clarifications, because the older code and the rules can look like they
disagree:

- **Reusable REST operations live on the helper clients, never on `TestContext`.**
  `TestContext` is for context-level concerns only. If a candidate helper makes an
  API call or builds a `rest_model.*Create` struct, it belongs on
  `ManagementHelperClient` / `ClientHelperClient`, not on `TestContext` and not as
  a package-level function.
- **No inline-struct-only helpers.** A "helper" whose entire body is constructing a
  request struct earns nothing — inline the struct at the call site.
- **The >3-uses bar is for pure computation only.** A pure helper used three or
  fewer times should be inlined.

A compliant package-level pure-computation helper: no API, no assertion, returns a
value, used several times, godoc'd.

```go
// usageByAttr returns the usage detail for the given role attribute, or nil
// if the list doesn't contain it.
func usageByAttr(items rest_model.RoleAttributeUsageList, attr string) *rest_model.RoleAttributeUsageDetail {
    for _, item := range items {
        if item.RoleAttribute != nil && *item.RoleAttribute == attr {
            return item
        }
    }
    return nil
}
```

## Assertions

The test does the asserting. Fixture-setup helpers return errors for the caller to
check; the legacy helpers that assert internally (`requireCreateEntity`,
`requireQuery`, and the like) hide where a failure came from. The shared helpers on
the client types follow the return-value pattern:

```go
identity, certCreds, err := managementHelper.CreateAndEnrollOttIdentity(false)
ctx.Req.NoError(err)
```

(For the rare asserting helper and its mandatory `t.Helper()` first line, see
[Two flavors of helper](#two-flavors-of-helper) above.)

**Assertion messages on anything non-obvious.** Say what should be true and why, so
a failure reads like a diagnosis:

```go
ctx.Req.Len(accessClaims.CertFingerprints, 2,
    "cert auth + CSR should have two cert fingerprints (auth cert + CSR cert)")
```

**Comments stating what behavior the test locks in**, especially for regression
guards or tests defending a design decision against future drift:

```go
// The intended behavior is that usage endpoints carry exactly the same
// management read permission as the existing *-role-attributes list
// endpoints. If the permission model ever diverges, this fails.
```

Helper functions and types get godoc comments, the same as production code.

## Use the typed API clients

This is the biggest single difference from the old style. Older tests drive
everything through `ctx.AdminManagementSession` with gabs and raw URLs. New tests
use the `edge-apis`-based helper clients:

```go
mgmtClient := ctx.NewEdgeManagementApi(nil)
adminCreds := ctx.NewAdminCredentials()
_, err := mgmtClient.Authenticate(adminCreds, nil)
ctx.Req.NoError(err)

clientApi := ctx.NewEdgeClientApi(nil)
```

From there, prefer the generated `rest_model` types and per-resource client
packages, with `util.Ptr` for pointer fields:

```go
createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
    EdgeRouter: &rest_model.EdgeRouterCreate{
        Name:           util.Ptr(eid.New()),
        RoleAttributes: &roleAttrs,
    },
}, nil)
ctx.Req.NoError(err)
```

The legacy `AdminManagementSession` helpers (`requireNewService`,
`requireNewServicePolicy`, `requireNewIdentity`, etc.) are still fine for building
**fixtures** quickly, especially for entity types the typed helpers don't cover
yet. But the thing actually **under test** should go through the typed clients or
explicit raw requests, not `validateEntityWithQuery`-style legacy validation.

## Raw HTTP — only when the wire format is the point

Drop down to raw HTTP when the test is specifically about status codes, error
bodies, headers, document shape (OIDC discovery, error codes, auth headers), or a
non-OpenAPI endpoint. Use `ctx.newAnonymousClientApiRequest()` /
`ctx.newAnonymousManagementApiRequest()` and decode the response into a small local
struct rather than asserting through gabs paths:

```go
// oidcErrorResponse matches the JSON structure returned by the zitadel/oidc
// library's WriteError handler.
type oidcErrorResponse struct {
    Error            string `json:"error"`
    ErrorDescription string `json:"error_description"`
}
```

If you must parse a gabs tree (some legacy helpers return one), convert it to a
typed local struct in one place and assert against that, instead of sprinkling
`.Path()` / `.S()` calls through the assertions:

```go
// decode once into a typed struct...
type usageRow struct {
    RoleAttribute string `json:"roleAttribute"`
    Count         int    `json:"count"`
}
var rows []usageRow
err := json.Unmarshal([]byte(container.S("data").String()), &rows)
ctx.Req.NoError(err)

// ...then assert against fields, not gabs paths
ctx.Req.Equal("sales", rows[0].RoleAttribute)
```

## Fixtures and data isolation

A **fixture** is an asset a test needs in place before it acts — an identity,
service, policy, router, etc. — as opposed to the behavior under test. Build
fixtures in the Arrange phase.

**Use fresh fixtures.** The controller and DB are shared across all subtests in a
test function, and other entities (default identities, blanket policies) already
exist. Create the fixtures each test needs inside that test, and make their names
and attributes globally unique so they can't collide with anything else. `eid.New()`
is the standard source of uniqueness:

```go
prefix := "idrau-" + eid.New() + "-"
attrBoth := prefix + "both"
...
```

Because the data store is shared, **don't assert against absolute counts of
unfiltered lists** — filter on your unique prefix instead:

```go
filter := url.QueryEscape(fmt.Sprintf(`id contains "%s" sort by id`, prefix))
```

## Dataflow tests

Tests that move traffic use the `dataflow` build tag and the same context, plus:

- `ctx.CreateEnrollAndStartEdgeRouter(...)` (or the tunneler / cfg-tweak variants)
  to run routers in-process.
- `ctx.AdminManagementSession.RequireCreateSdkContext()` for SDK identities; always
  `defer Close()` on contexts and listeners.
- **No sleeps.** Wait on channels with explicit timeouts:

  ```go
  select {
  case info := <-serverInfoC:
      ctx.Req.Equal(expected, info.id, "dialer identity ID should match")
  case <-time.After(5 * time.Second):
      ctx.Req.Fail("timed out waiting for server to receive connection")
  }
  ```

- For controller events, define a small collector that implements the event
  handler interface and pushes onto a buffered channel, register it, then wait on
  the channel with a deadline:

  ```go
  type circuitCollector struct {
      events chan *event.CircuitEvent
  }

  func (c *circuitCollector) AcceptCircuitEvent(e *event.CircuitEvent) {
      c.events <- e
  }

  // arrange: register the collector
  fc := &circuitCollector{events: make(chan *event.CircuitEvent, 50)}
  dispatcher.AddCircuitEventHandler(fc)
  defer dispatcher.RemoveCircuitEventHandler(fc)

  // ... act, then assert with a deadline
  select {
  case e := <-fc.events:
      ctx.Req.NotNil(e.FailureCause)
      ctx.Req.Equal(string(expectedCause), *e.FailureCause)
  case <-time.After(5 * time.Second):
      ctx.Req.Fail("timed out waiting for circuit event")
  }
  ```

- Wait for a service to become dialable by watching for its terminators rather than
  polling/sleeping.

## Things to avoid (old patterns)

- Inline function closures assigned to variables inside a test body (subtest bodies
  are fine).
- Fixture-setup helpers that assert internally instead of returning errors.
- Asserting helpers that omit `ctx.T().Helper()` on the first line, or asserting
  helpers written where inlining would have been just as readable.
- Reusable REST operations written as test-file functions or hung off `TestContext`
  instead of living on the helper clients.
- Helpers whose only job is constructing an inline struct.
- gabs path navigation in assertions (`RequireChildWith`,
  `RequireGetNonNilPathValue` scattered through test bodies).
- Legacy entity validation helpers (`validateEntityWithQuery`,
  `validateEntityWithLookup`, `entity.validate(ctx, json)`) for new endpoints.
- Bare assertions with no message where the expectation isn't self-evident.
- Asserting on unfiltered list contents or counts.
- `time.Sleep` to wait for async behavior.
- Dependent subtests placed as siblings instead of nested.
