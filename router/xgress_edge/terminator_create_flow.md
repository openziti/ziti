# Terminator Create Flow

## Overview

This documents the complete flow of terminator creation across the SDK, router, and
controller, with emphasis on timing and state transitions.

## Sequence

```
SDK                          Router                              Controller
 |                            |                                   |
 |-- bind msg -------------> |                                   |
 |                            |-- checkForExistingListenerId()    |
 |                            |   (findMatchingEvent on event     |
 |                            |    loop adds to terminators map)  |
 |                            |                                   |
 |                            |-- terminator state: ESTABLISHING  |
 |                            |                                   |
 |                            |-- StateConnected reply ---------> |
 |  <------------------------ |   (SDK unblocks here, before      |
 |                            |    controller is contacted)       |
 |                            |                                   |
 |                            |   if replaceExisting:             |
 |                            |     NotifyEstablished to SDK      |
 |                            |     go queuePostCreateInspect     |
 |                            |     (done - no controller call)   |
 |                            |                                   |
 |                            |   if new:                         |
 |                            |     EstablishTerminator queued    |
 |                            |     go queuePostCreateInspect     |
 |                            |                                   |
 |                            |-- (event loop tick) -----------   |
 |                            |   EstablishTerminator is queued   |
 |                            |   synchronously so it is usually  |
 |                            |   processed before the inspect    |
 |                            |   event (queued from goroutine)   |
 |                            |                                   |
 |                            |-- CreateTerminatorV2Req --------> |
 |                            |                                   |-- Dispatch(cmd)
 |                            |                                   |-- DB.Update():
 |                            |                                   |     store.Create()
 |                            |                                   |     TerminatorCreated fires
 |                            |                                   |       (synchronous, in txn)
 |                            |                                   |       queues to RouterMessaging
 |                            |                                   |-- Dispatch returns
 |                            |                                   |
 |                            |                                   |-- ch.Send(response)
 |                            |   <---------------------------------|
 |                            |                                   |
 |                            |-- HandleCreateTerminatorResponse  |
 |                            |     markEstablished() queued      |
 |                            |     NotifyEstablished() called    |
 |                            |       (sends BindSuccess inline)  |
 |                            |                                   |
 |  <-- BindSuccess --------- |                                   |
 |                            |                                   |
 |  <-- inspect request ----- |   (evaluatePostCreateInspects     |
 |  --- inspect response ---> |    runs on each event loop tick)  |
 |                            |-- inspectResponseEvent.handle()   |
 |                            |     confirms terminator valid     |
 |                            |                                   |
 |                            |-- (event loop tick) -----------   |
 |                            |-- markEstablishedEvent.handle()   |
 |                            |     state: ESTABLISHED            |
 |                            |     if >30s since bind:           |
 |                            |       queue 2nd post-create       |
 |                            |       inspect (re-verify SDK)     |
 |                            |                                   |
 |                            |                                   |-- (RouterMessaging loop)
 |                            |                                   |-- sendTerminatorValidationRequest
 |                            |                                   |   (only includes terminators
 |                            |                                   |    with CreatedAt > 5s ago;
 |                            |                                   |    loop ticks every 30s or
 |                            |                                   |    on event, so delay is 5-35s)
 |                            |   <---------------------------------|  ValidateTerminatorsV2Request
 |                            |                                   |     (PostCreate=true)
 |                            |-- validateTerminator              |
 |                            |     InspectTerminator             |
 |                            |       postCreate: confirms        |
 |                            |       terminator present in map   |
 |                            |       (no SDK inspect needed,     |
 |                            |        router inspect covers it)  |
 |                            |-- sends response ----------------> |
 |                            |                                   |-- terminatorValidationRespReceived
 |                            |                                   |     valid: remove from set
 |                            |                                   |     invalid: queue delete
```

## Key Timing Details

### SDK unblocks before controller is contacted

The router sends `StateConnected` back to the SDK immediately in `processBindV2`,
before queuing `EstablishTerminator`. The SDK's `SendForReply` on the bind message
returns here. The controller hasn't been contacted yet.

### SDK has a 1-minute establishment timeout

After `listen()` returns (on StateConnected), the `multiListener.forward()` method
monitors the `established` flag. If BindSuccess doesn't arrive within 1 minute,
the listener is closed and removed from `multiListener.listeners`, allowing
`needsMoreListeners()` to trigger a replacement bind.

### TerminatorCreated fires during the DB transaction

The `TerminatorCreated` event listener is registered on the bolt store via
`AddEntityEventListenerF`. It fires synchronously during the `DB.Update()` call,
before `Dispatch()` returns to the handler. This means the event is queued to
`RouterMessaging` before the create response is sent back to the router.

### Response delivery is not guaranteed

After `Dispatch()` returns, the controller sends the response via `ch.Send()`.
If this fails (channel full, closed, etc.), the error is only logged. There is
no retry mechanism. The router will eventually retry the create after a 30-second
`operationActive` timeout.

### markEstablished requires receiving the response

The router only calls `markEstablished` (transitioning from ESTABLISHING to
ESTABLISHED) when it receives the success response from the controller. If the
response is never received, the terminator stays in ESTABLISHING state.

## Post-Create Validation

### Router-side (post-create inspect)

Queued from `processBindV2` (via a goroutine) after the bind response is sent,
for both the `replaceExisting` and new terminator paths. For the new terminator
path, `EstablishTerminator` is queued synchronously first, so it is typically
processed before the inspect event. The inspect sends a `ConnInspectRequest` to
the SDK via `TrySend` (non-blocking) and verifies the SDK still has the bind.
If invalid, the router queues a remove.

A second post-create inspect is queued in `markEstablishedEvent.handle()` if
establishment took >30 seconds. This covers the case where the initial inspect
confirmed validity but the SDK subsequently timed out waiting for BindSuccess
(the SDK has a 1-minute establishment timeout). The re-inspect catches stale
validity from the first check.

### Controller-side (TerminatorCreated validation)

The controller queues newly created terminators for validation via
`RouterMessaging`. The messaging loop runs every 30 seconds (or when an event
arrives) and only includes terminators whose `CreatedAt` is more than 5 seconds
old, so the effective delay is 5–35 seconds. The request is sent with
`PostCreate=true`, which tells the router to just confirm the terminator is
present in its map — the router's own post-create inspect handles SDK-level
validation. Invalid or missing terminators are deleted.

### Why both are needed

The router-side inspect catches cases where the SDK has timed out or closed the
bind before or after establishment. The controller-side validation catches
orphans where the controller wrote a terminator to the DB but the router lost
track of it (e.g., SDK channel closed, router restart, response lost).

## Orphan Terminator Scenarios

A terminator becomes orphaned in the controller DB when:

1. Controller writes terminator to DB (TerminatorCreated fires)
2. Something prevents the router from keeping the terminator:
   - SDK channel closes before create response arrives (host killed)
   - Router removes terminator from its map (channelClosedEvent)
   - Router restarts, losing in-memory state
3. The controller's delete request fails or is never sent

The controller-side post-create validation (TerminatorCreated -> 5–35s delay ->
ValidateTerminatorsV2Request) is the primary mechanism for catching these orphans.

## On Router Connect

When a router connects to the controller, `ValidateTerminators(r)` runs. This
queries ALL terminators for that router from the DB and queues them for
validation via `ValidateRouterTerminators`. This is a one-shot catch-up that
handles terminators orphaned while the router was disconnected. However, it
does not catch terminators created after the router connects.
