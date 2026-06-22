# Router Data Model Race Conditions

## Context

Investigation of missing terminators in sdk-hosting-test. 14 out of 15000 expected terminators were
missing. 6 identities on router-ap-0 were told they lost bind access to some services during a full
RDM state replacement, causing permanent terminator loss (SDK treats "bind access lost" as
`RetryNotRetriable`).

Timeline:
- 23:13:10 - CLUSTER_NO_LEADER errors
- 23:13:11.931 - Full RDM state received from ctrl3 (index 205116, replacing old index 202533)
- 23:13:11.990 - RDM replacement starts
- 23:13:11.994 - RDM replacement complete
- 23:13:12.016-018 - Bind access lost events for 6 identities across 5 services

The access loss was TEMPORARY - current RDM at index 210874 shows correct data for all affected
identities. The router likely re-subscribed to a different controller with a correct sender model.

---

## Race Condition 1: SyncAllSubscribers was async (FIXED)

### Description

`SyncAllSubscribers()` only queued a `syncAllSubscribersEvent` to the events channel (async). After
`SetRouterDataModel` stored the new model and returned, the pool worker became free to process
incremental `ApplyChangeSet` calls. Meanwhile, the `processSubscriberEvents` goroutine was still
processing the `syncAllSubscribersEvent` on a separate goroutine.

This meant incremental changes could modify the model's ServiceAccess data concurrently with
`checkForChanges` iterating it, potentially causing false `ServiceAccessLostEvent` notifications.

### Fix

Made `SyncAllSubscribers()` synchronous by adding a `completeNotify` channel that blocks until
the sync event is fully processed.

### Relevance to incident

Likely insufficient to explain this specific incident since only terminator updates (not policy
changes) should have been happening. Terminator updates don't flow through the RDM.

---

## Race Condition 2: Controller BuildAll / Entity Constraint Registration Gap

### Description

In `InstantStrategy.Initialize()` (sync_instant.go:139-244), the ordering is:

```
1. Line 140:      Create sender model
2. Line 155:      BuildAll (reads DB in View/MVCC snapshot at time T_build)
3. Line 161:      Start handleRouterModelEvents goroutine + NewListener
4. Lines 163-241: Register entity constraints (addToChangeSet handlers)
5. Line 242:      Register tx complete listener (completeChangeSet)
```

`raft.NewRaft()` is called BEFORE `Initialize()`. The Raft subsystem is already running and can
apply NEW log entries (index > startIndex) via `FSM.Apply` at any time.

**Window 1: Between BuildAll and entity constraint registration (steps 2-4)**

If a Raft entry commits to the DB after BuildAll's MVCC snapshot time but before entity constraints
are registered:
- The data is written to the DB by `FSM.Apply`
- BuildAll's MVCC snapshot doesn't see it
- Entity constraints not registered => no `addToChangeSet` fires => no events generated
- Sender model **permanently** misses this data

**Window 2: Between entity constraint and tx complete listener registration (steps 4-5)**

If a Raft entry goes through while entity constraints are registered but `completeChangeSet` isn't:
- Entity constraints fire => `addToChangeSet` accumulates events in `strategy.changeSets[index]`
- Transaction completes but `completeChangeSet` not registered => events sit in the map
- When the NEXT transaction completes (after step 5), `completeChangeSet` runs:
  ```go
  for k := range strategy.changeSets {
      if k <= index {
          delete(strategy.changeSets, k)  // DELETED without being applied!
      }
  }
  ```
- The orphaned changeSets are silently cleaned up

### Impact

If ctrl3's sender model was built during startup while Raft entries containing identity-to-policy
associations were being applied, the sender model would permanently miss those associations.

When ctrl3 sends a full state snapshot to a router:
- Identities would be present
- Service policies would be present
- But `ServicePolicyChange` (RelatedIdentity) events linking specific identities to policies would
  be missing

The router would build its RDM from this incomplete state, `checkForChanges` would find services
missing from the new ServiceAccess, and fire `ServiceAccessLostEvent`.

### Validation gap

`ValidateServicePolicies` (sync_instant.go:1179-1197) does NOT validate `SenderIdentity.ServicePolicies`
associations. It only validates the policy's Services and PostureChecks maps. So this inconsistency
would not be caught by validation.

### Open questions

- Was ctrl3 recently (re)started before the incident?
- Were there any Raft entries being applied during ctrl3's `Initialize()` call?
- The CLUSTER_NO_LEADER at 23:13:10 is suspicious - did it cause a controller restart?

---

## Race Condition 3: cmap IterCb during getDataStateAlreadyLocked

### Description

`getDataStateAlreadyLocked` (router_data_model_sender.go:411-541) builds the full state by iterating
cmaps using `IterCb`. While the EventCache lock prevents new events from being stored/applied, cmap's
`IterCb` uses per-shard RLock (not a global snapshot).

If there were any concurrent writer to the cmaps outside the EventCache lock path, items could be
skipped during iteration.

### Assessment

The EventCache lock should prevent this since `ApplyChangeSet` -> `EventCache.Store` acquires the
same lock. Unless there's a path that modifies `SenderIdentity.ServicePolicies` without going through
the EventCache (e.g., synthetic events, BuildAll running concurrently), this shouldn't happen.

**Likelihood: Low** unless BuildAll and event application overlap.

---

## Additional Note: BoltDbFsm.startIndex never set

`BoltDbFsm.startIndex` (the struct field at fsm.go:88) is never assigned. In `Init()`, the local
variable `startIndex` is loaded and set to `self.index`, but the struct field stays at 0. So
`GetStartIndex()` -> `GetStartRaftIndex()` always returns 0, and `RaftIndexProvider` initializes
with index 0.

This means `BuildAll` uses `indexProvider.CurrentIndex()` = 0, and `SetCurrentIndex(0)` on the
EventCache is a no-op. All subsequent events are accepted (since any raft index > 0). This appears
to be a bug, though it may not have functional impact since events arrive in order.

---

## Architectural Concern: SDK retry behavior

When the router reports a bind access loss (even a temporary one due to RDM replacement), the SDK
receives `RetryNotRetriable` and permanently closes the listener. The SDK polls the controller for
service changes independently, so it doesn't know the loss was temporary.

Suggested improvement: defer sending retry hints to the SDK until the SDK is getting service updates
from the router using the same subscriber mechanism, so temporary blips during RDM replacement don't
cause permanent terminator loss.
