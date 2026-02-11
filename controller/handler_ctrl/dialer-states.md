# CtrlDialer State Machine

## States

| State         | Description                                                                                                             |
|---------------|-------------------------------------------------------------------------------------------------------------------------|
| **(none)**    | Router has no entry in the `states` map. Either unknown, already connected, or no matching endpoints.                   |
| **NeedsDial** | Router needs a connection. Sitting on the retry queue with a scheduled `nextDial` time.                                 |
| **Dialing**   | A dial attempt has been submitted to the worker pool and is in progress.                                                |
| **Connected** | Dial succeeded. Remains tracked until the fast-failure check confirms the connection survived past `FastFailureWindow`. |

## Address Rotation

Each `routerDialState` tracks a list of ctrl channel listener addresses and a current
index (`addrIndex`). When a router advertises multiple addresses, the dialer rotates
through them on each failure so that an unreachable address does not block attempts to
the others.

- On **dial failure** (including pool-full): `addrIndex` advances to the next address.
- On **normal disconnect** (connection survived past `FastFailureWindow`): `addrIndex`
  resets to 0 so the next dial cycle starts from the first address.
- On **evaluate**: the address list is refreshed from the datastore and `addrIndex` is
  clamped if the list shrank.

## State Transitions

```
                     ┌─────────────────────────────────────────────────┐
                     │                                                 │
                     ▼                                                 │
  ┌──────┐    ┌───────────┐    ┌─────────┐    ┌───────────┐    ┌──────┴─────┐
  │(none)│───▶│ NeedsDial │───▶│ Dialing │───▶│ Connected │───▶│  (removed) │
  └──────┘    └───────────┘    └─────────┘    └───────────┘    └────────────┘
                  ▲    │           │               │
                  │    │           │               │
                  │    └───────────┘               │
                  │     dial failed /              │
                  │     pool full                  │
                  │                                │
                  └────────────────────────────────┘
                   connection lost (normal or fast failure)
```

## Triggers and Actions

### Entry into tracking — (none) -> NeedsDial

A `routerDialState` is created and added to the `states` map when any of these occur
and the router has matching ctrl channel listener endpoints:

| Trigger                   | Source                                                                                |
|---------------------------|---------------------------------------------------------------------------------------|
| `routerDisconnectedEvent` | Controller-initiated channel close handler, or `RouterDisconnected` presence callback |
| `routerUpdatedEvent`      | Router entity updated or created in the datastore                                     |
| `scan()`                  | Initial scan at startup, or periodic hourly scan                                      |

New entries from `scan()` have their `nextDial` spread across a time window (50ms apart)
to avoid thundering herd on controller startup.

### NeedsDial -> Dialing

When `evaluateRetryQueue()` pops a state whose `nextDial` has arrived,
`evaluateDialState()` verifies the router still needs a dial and submits the work
to the `dialPool` via `QueueOrError()`. The `dialActive` atomic flag prevents
duplicate submissions.

### Dialing -> NeedsDial (dial failed)

When `doDial()` completes with an error, a `dialResultEvent` is sent.
The event handler calls `dialFailed()`, which applies exponential backoff with jitter:

```
factor = RetryBackoffFactor + rand(-0.5, 0.5)   (clamped to min 1.0)
retryDelay = retryDelay * factor
retryDelay = clamp(retryDelay, MinRetryInterval, MaxRetryInterval)
nextDial = now + retryDelay
addrIndex = (addrIndex + 1) % len(addresses)
```

The address is rotated so the next attempt tries a different ctrl channel listener.
The state is pushed back onto the retry queue.

If the pool is full (`QueueOrError` returns error), the same backoff is applied
without ever entering Dialing.

### Dialing -> Connected (dial succeeded)

When `doDial()` completes with no error, a `dialResultEvent` is sent.
The event handler calls `dialSucceeded()`, which:
- Sets status to Connected and records `connectedAt`
- **Does not reset `retryDelay`** (preserved for fast-failure detection)
- Schedules a re-check at `now + FastFailureWindow` on the retry queue

### Connected -> (removed) (connection survived)

When `evaluateDialState()` processes a Connected state after `FastFailureWindow`
has elapsed and finds the router is still connected via `GetConnectedRouter()`:
- Resets `retryDelay` and `dialAttempts` to zero
- Removes the entry from the `states` map

### Connected -> NeedsDial (fast failure)

If a `routerDisconnectedEvent` arrives while in Connected state and
`time.Since(connectedAt) < FastFailureWindow`, `connectionLost()` treats this as
a fast failure and calls `dialFailed()` — continuing the backoff progression rather
than resetting it. This prevents rapid-fire dial cycles when a router keeps
accepting TCP but immediately rejecting the channel.

Alternatively, if `evaluateDialState()` finds the router is no longer connected
after the FastFailureWindow check, it calls `connectionLost()` which applies the
same logic.

### Connected -> NeedsDial (normal disconnect)

If `connectionLost()` is called and either:
- The state is not Connected, or
- `time.Since(connectedAt) >= FastFailureWindow`

Then the backoff is fully reset (`retryDelay = 0`), the address index is reset to 0,
and `nextDial` is set to `now` for an immediate retry. The assumption is that a
long-lived connection dropping is a transient network issue, not a persistent rejection.

### Any state -> (removed) (router deleted)

A `routerDeletedEvent` unconditionally removes the entry from the `states` map.

### Any state -> (removed) (no endpoints)

If `evaluateDialState()` or `routerUpdatedEvent` finds that `getRouterIdEndpointsNeedingDial()`
returns no addresses (router disabled, connected elsewhere, or no matching listener groups),
the entry is removed from the `states` map.

### External connection — routerConnectedEvent

When a router connects via its own outbound dial (not controller-initiated), the
`routerConnectedEvent` resets the state to Connected with zeroed backoff. The entry
stays in the map briefly until the next `evaluateDialState()` confirms it is connected
and removes it.

## Configuration

| Field                | Default | Description                                                                     |
|----------------------|---------|---------------------------------------------------------------------------------|
| `MinRetryInterval`   | 1s      | Floor for backoff delay                                                         |
| `MaxRetryInterval`   | 5m      | Ceiling for backoff delay                                                       |
| `RetryBackoffFactor` | 1.5     | Multiplier per failure (jittered +/- 0.5)                                       |
| `FastFailureWindow`  | 5s      | Connections dying within this window trigger backoff instead of immediate retry |
| `DialDelay`          | 30s     | Initial delay before the dialer starts after controller boot                    |
