# Bind Message Channel Ordering Analysis

## Overview

With multi-underlay (separate control and data channels), messages sent on different
channels have no ordering guarantee relative to each other. This document analyzes
the channel usage for bind-related messages and identifies race condition potential.

## Channel Usage Map

### Router → SDK

| Message | Channel | Method | Code |
|---|---|---|---|
| StateConnected (bind reply) | **Data** | SendAndWaitForWire | listener.go:997 `GetDefaultSender()` |
| BindSuccess | **Control** | Send | fabric.go:137 `GetControlSender()` |
| ConnInspectRequest (post-create) | **Control** | TrySend | hosted.go:628 `GetControlSender()` |
| ConnInspectRequest (validate) | **Control** | SendForReply | fabric.go:156 `GetControlSender()` |
| StateClosed (bind error) | **Data** | TrySend | hosted.go:482 `GetDefaultSender()` |
| Dial | **Control** | SendForReply | dialer.go:171 `GetControlSender()` |
| StateClosed (conn close) | **Data** | SendAndWaitForWire | via `SendState → GetDefaultSender()` |
| Data | **Data** | Send | fabric.go:423 `GetDefaultSender()` |

### SDK → Router

| Message | Channel | Method | Code |
|---|---|---|---|
| Bind | **Control** | SendForReply | hosting_conn.go:476 `GetControlSender()` |
| Unbind | **Control** | SendAndWaitForWire | hosting_conn.go:503 `GetControlSender()` |
| ConnInspectResponse | **Control** | Reply | hosting_conn.go:195 `GetControlSender()` |
| DialSuccess/Failed | reply (matches sender) | | |
| StateClosed (conn close) | **Data** | SendAndWaitForWire | via `SendState → GetDefaultSender()` |
| Data | **Data** | | via `GetDefaultSender()` |

## Reply Mechanism

`SendForReply` matches replies by sequence number via a single global waiter map
shared across all underlays (`channel/multi.go`). Replies are matched regardless
of which underlay they arrive on. So the bind request (sent on control) and its
StateConnected reply (sent on data) are correctly matched.

## Race Condition Analysis

### 1. ConnInspectRequest vs StateConnected — Safe (no bug)

The post-create inspect is queued via `go queuePostCreateInspect(terminator)` right
after StateConnected is sent. It goes through the event loop + `evaluatePostCreateInspects`
tick interval. In practice it arrives much later. But there's no guarantee — if the
data channel is congested and control isn't, the inspect could arrive first.

**Impact if inspect arrives first:** Safe. The `edgeHostConn` is registered in the
mux *before* `listen()` is even called (factory.go:201). So the inspect finds the
sink, calls `handleInspect`, gets `ConnTypeBind` — correct answer. The SDK's
`listen()` is still blocked on `SendForReply` waiting for StateConnected, but
that's independent.

### 2. BindSuccess vs StateConnected — Safe (no bug)

BindSuccess goes on control, StateConnected goes on data. BindSuccess could arrive
first. But the handler just sets `conn.established.Store(true)` — no dependency on
StateConnected having been received.

### 3. StateClosed (bind error) vs StateConnected — Safe (same channel)

If bind access is lost during setup (listener.go:1027), the close is sent on data
(`GetDefaultSender().TrySend`), same channel as StateConnected. FIFO ordering within
the data channel means StateConnected arrives first.

### 4. Dial vs bind lifecycle — Naturally ordered

Dials can only happen after the terminator is established in the controller, which
is much later than bind processing. Not a real race.

### 5. Data vs StateConnected — Naturally ordered

Data can only flow after a dial succeeds, which requires controller establishment.
The architectural sequencing (bind → establish → dial → data) prevents this race
regardless of channel assignment.

## Current Comment Analysis

listener.go:996 says:
```
// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
```

This concern about ordering with data doesn't hold with multi-underlay anyway —
if StateConnected were on control and data on data, they'd be independent paths.
But the concern is moot because data can't arrive until a dial completes (which
requires controller establishment), so there's a natural architectural ordering
that makes the channel choice irrelevant for data ordering.

The concern about StateClosed ordering IS valid within the data channel — bind-error
closes are sent on data after StateConnected, so FIFO ordering within the data
channel keeps them ordered. But bind-error closes happen before any data flows,
so there's nothing else on data to race with.

## Recommendation: Use Control Channel for Bind Lifecycle

Most bind lifecycle messages already use the control channel:
- Bind request → control
- Unbind → control
- BindSuccess → control
- ConnInspectRequest/Response → control
- Dial → control

Only two use data:
- StateConnected (bind reply) → data
- StateClosed (bind error) → data

Moving these to control would:
- Give strict FIFO ordering for all bind lifecycle messages on one channel
- Eliminate cross-channel ordering dependencies
- Make the ordering correct by design rather than by accident of timing
- Keep dial-related closes on data (where they must be to not race with data)

The current code has no bugs from these races — the handlers are safe (inspect
before bind works because edgeHostConn is already in the mux), and the timing
makes races impractical (data before StateConnected is impossible because dial
hasn't happened). The change would be a correctness/clarity improvement.
