# Sopholeth roadmap

The next phase delivers three things: the first public network, a web
application that makes it useful to people, and a simulation that tests its
longer-term premise.

The [core principles](core-principles.md) constrain production behavior.
The [architecture](architecture.md) describes the current implementation.
This roadmap describes work still required; it is not a list of shipped
guarantees.

## Current foundation

The Go node already implements anonymous HTTP put/get/list, in-memory TTL
storage, enclave gossip, replication acknowledgments, signed discovery and
caching, WebSocket attachments, and an embedded MCP interface. The `soph` CLI,
initial `soph serve`, static viewer, and separate dashboard also exist.

The omega operator lifecycle is implemented, including encrypted custody,
publication, renewal, rotation, backup checking, and verified Vercel hosting.
The hosted rehearsal and Kraid service invocation succeeded. Node/CLI consumers
still use the old discovery path; their migration to the HTTPS trust client
and public bundle is unfinished. The public network has not been activated.

## First public network

The immediate objective is **three public roots running for testing**, using
Linux standalone nodes in the `default` enclave. The
[revised network plan](public-network-plan.md) separates existing functionality,
actual bring-up prerequisites, and repairs to make on the running network.

Participation is permissionless: any compatible node can bootstrap, join, and
gossip, and writes have no authenticated author. Omega endorses bootstrap
entry points. Peer bookkeeping and observed replication counts do not imply
controlled admission, trusted voters, or consensus.

The SYNC-storm fix (#150) is implemented: explicit peer-list requests receive
one-way announcements that cannot trigger another reply. The remaining work is:

1. Connect nodes and the Linux CLI to the existing HTTPS trust client and
   bundle, including authenticated bootstrap origins, expiry, cached fallback,
   and saved-profile refresh. Preserve the existing anonymous data path.
2. Write the actual host/DNS/TLS/service runbook and deploy three roots from a
   reviewed, tested commit. Reuse omega's implemented custody and renewal
   procedures. Record shared failure domains if roots share a host.
3. Verify put/get/list, healthy replication, TTL expiration, and an additional
   node joining without approval. Then use the running network to test root
   loss, slow peers, restarts, capacity, and remaining audit findings.

Peer-record defects (#211/#213), ACK accounting (#164), slow-peer delivery
(#212), and related races/lifecycle defects remain real work. Scope each fix to
observed protocol behavior; do not introduce membership authorization or writer
identity to satisfy an audit. They are not collectively a prerequisite for
starting useful network tests. A failure that prevents basic joining or healthy
replication on the actual deployment takes priority.

Native Windows discovery remains a follow-up deliverable; initial Linux
bring-up need not wait for it. MCP, dashboard deployment, full transient-writer
participation, and viewer polish are deferred. Keep unused endpoints excluded
from the initial deployment and test them before enabling their participation
mode. The existing viewer can observe the network as its streaming path is
included; retain the current aesthetic and named-network behavior.

## Before public alpha

This is broader follow-up work after the initial test network is running.
Earlier launch labels refer to this broader scope. The network plan defines
what is required for first bring-up; this list is not a second prerequisite
checklist for that milestone.

- Work through replication, peer bookkeeping, capacity, deduplication, replay,
  and shutdown defects using small, targeted reproductions. Preserve the
  current local-lifetime and opaque-value contract.
- Harden public resource handling: storage overhead, key/value limits,
  listing and expiry work, rate limiting, HTTP deadlines and connections, and
  stream behavior (#217–#221). Deployment bounds used for testing do not close
  these issues or establish denial-of-service resistance.
- Complete native Windows public-client state storage and integration tests
  before claiming Windows `soph join`, profile renewal, and `soph serve` support.
- Validate WebSocket/transient behavior and safeguards before enabling it.
  Dashboard and MCP work retain their own scope.
- Expand fault, churn, partition, capacity, and sustained-load testing as
  problems are fixed. Reuse the [burn-in harness](../test/burnin/README.md) where
  useful; record workloads, versions, resource use, and observed limits.
- Complete release-pipeline review/test gates and artifact verification
  (#223/#224) before distributing a general release. The test deployment uses
  explicitly recorded builds rather than floating image tags.
- Publish accurate support limits, operator recovery/upgrade instructions,
  and the distribution terms already flagged for review in `LICENSE`.

Longer testing happens on the running network and disposable local clusters.
It does not promise independent trusted replicas, global ordering, durable
payload recovery, or authenticated writers. Application-level signatures and
identity remain client concerns.

## Demo web application

Build a small temporary conversation application: chat that behaves more
like speech than a permanent transcript.

The first experience should support invite-based rooms, live messages with
visible remaining lifetimes, optional client-side encryption, and rooms that
expire when participants stop refreshing them. Participants need no accounts.
The interface should explain that readers can still copy or record messages.

Keep the application protocol above Sopholeth:

- Give each message its own key.
- Use temporary room metadata to discover live messages.
- Put timestamps, signatures, and encryption envelopes inside client payloads.
- Handle concurrent metadata updates explicitly, using separate branch keys
  or a merge scheme that does not rely on atomic overwrite.
- Test slow readers, missing values, expiry, and interrupted connectivity.

**Exit:** two people can join a room, converse through the public network,
leave, and later find no live transcript served by the application. Publish
the client protocol so another application can reproduce the interaction.

MCP handoffs and presence signals can be evaluated later; they do not gate
the public network or demo. Keep agent orchestration and application identity
outside the node.

## Probe simulation

Build a separate simulation that consumes Sopholeth's primitive. Begin with
a small live-network scenario that writes and reads expiring signposts.
For larger and longer experiments, use a deterministic discrete-event model
checked against the current implementation's behavior.

Model distance and communication delays, local clocks, intermittent contact,
probe mortality and dormancy, descendant lineages, replication costs,
bandwidth and storage limits, and regional enclaves. The simulator may own
a global clock; simulated probes must act on local observations.

A first signpost protocol can carry observations, routes, hazards, signer
lineage, references to earlier observations, and refresh history. Visitors
validate what they understand, apply their own conflict policy, and choose
whether to renew or carry a signpost onward. Nodes still store opaque bytes.

Measure:

- Useful information survival versus stale-information retention.
- Refresh cost and TTL choices under delayed or lost communication.
- Information movement relative to the expansion frontier.
- Lineage divergence, conflicting observations, and loss of contact.
- Storage exhaustion and the cost of topology knowledge.

Visualize signpost lifetimes, refresh events, communication horizons, and
knowledge fading across disconnected regions.

**Exit:** repeatable runs and published assumptions explain what was learned.
Hypothetical protocol changes are explicitly versioned experiments, separate
from production behavior.

## Protocol independence and later research

Capture the current wire contract in a normative specification, with golden
fixtures and a black-box conformance runner. Cover HTTP and WebSocket gossip,
TTL, quorum outcomes, discovery, HMAC canonicalization, and error behavior.
A second implementation should target that contract instead of reconstructing
it from the Go source.

Use simulation results to evaluate longer TTL representations, bounded local
topology, store-and-carry transport, protocol evolution, and lineage trust.
The existing terrestrial discovery system is not a galactic trust model.
Production changes require evidence and compatibility decisions.

For each release, publish the behavior changes, supported interfaces,
validation artifacts, known limits, and operator migration steps. Preserve
historical evidence without presenting it as a current runbook.
