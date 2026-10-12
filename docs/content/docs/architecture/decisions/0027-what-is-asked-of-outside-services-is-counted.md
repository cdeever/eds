---
title: "ADR-0027: What Is Asked of Outside Services Is Counted"
weight: 27
---

# ADR-0027: What EdS Asks of Other People's Services Is Counted, a Minute at a Time, and nowplayd Writes Its Counts to the Log Store Itself

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the counts have been seen on the dashboard; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-12 |
| **Scope** | Counting the calls `npagent` and `nowplayd` make to services that are not EdS's; how those counts reach the tenant's log store from each; the dashboard that reads them. |
| **Extends** | [ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/) and [ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/): the agent's counts are one more event. A workload gets a route of its own, which neither gave it. |
| **Related** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), whose recognizer is the service least entitled to be leaned on; [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/), [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/); Deevnet ADR-0027 (the tenant log store) and Deevnet ADR-0024 (dashboards) |

---

## Context

EdS asks things of services that are not its own. The turntable's recognizer
is Shazam's, through an endpoint nobody offered. The owner's collection is
read from Discogs. Spotify's Web API is polled. Covers are looked up at
Deezer, the iTunes Search API, MusicBrainz and the Cover Art Archive.

Each has a limit, published or not, and each client was written to keep
inside it. Nothing shows whether it does. The only traces are two events for
when something has already gone wrong: `spotify.refused` and
`turntable.recognizer_failed`. How many calls were made in an hour of
records is not written down anywhere.

The calls are made in two places:

| Service | Asked from | A route to the log store today |
|---|---|---|
| the recognizer, Spotify | `npagent`, on the agent Pi | yes: the Pi is a device, and a log line with an `event` is carried there ([ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/)) |
| Discogs, the cover catalogs | `nowplayd`, on the workload | none: a workload is not a device and may not publish under `log/` |

The platform has no metrics store. Its own advice is to graph numbers out of
log lines, which is how the stand's dashboard already works.

## Options considered

### What is written down

**A line for every call.** Everything is there, and the store can count
lines. Spotify's driver alone asks 8,640 times a day when it is on, and the
agent's events share a queue of thirty-two with the lines about tracks and
drivers.

**A summary a minute for each service.** Calls, how many were refused for
coming too often, how many failed, the slowest, and what the service said
was left of its allowance. A minute with no calls writes nothing. The store
adds the numbers up. A single call's details are lost; the journal still
has whatever the client logged about it.

**Counters read by a scraper.** There is nothing to scrape them into.

### How nowplayd's counts get there

**Through the broker, as a device does.** A workload's account cannot
publish under `log/`, by the substrate's design, and nowplayd is not a
device.

**The substrate's way for a workload**: post lines to the store with the
tenant's ingest token, which writes to the tenant's workload partition and
nothing else. `kit.env` already hands nowplayd the endpoint and the token.
No EdS workload has used them.

**All of nowplayd's log, the same way.** More than was asked for, and a
decision of its own: what a workload's log is for, and how much of it.

### Where the counting is done

**In each client.** Seven places to get it right, and an eighth the next
time a client is written.

**In the HTTP client they are all given.** One place. It sees redirects and
failures as well, and names the service from the host. The recognizer is a
program and not an HTTP client, so it is wrapped where the agent makes it.

## Decision

**Every call to an outside service is counted, and once a minute each
service that was asked anything gets one line, the event `api.calls`.**

```json
{"event":"api.calls","api":"discogs","calls":3,"limited":0,"failed":0,"slowest_ms":412,"mean_ms":280,"remaining":57}
```

- **`limited` is a 429.** `failed` is no answer, an error, or a refusal.
  "Not found" is an answer, and the commonest one a cover archive gives.
- **`remaining` is the least the service said was left** in that minute,
  for a service that says. Discogs does, in every answer.
- **A service's pictures are counted apart** from its API
  (`discogs-images`): the allowance is the API's, and a count swollen with
  pictures would say nothing about it.
- **One clip the recognizer is asked about is one call.** The recognizer
  says whose service it asked, in its answer, and the count is filed under
  that. The driver still knows only "a WAV in, a name out"
  ([ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)).
- **It counts, and limits nothing.** It does not slow a caller or stop one.
  Each client keeps its own service's rules; this is how to see that it
  does.

**`npagent` reports the line as an event**, like its others.

**`nowplayd` posts the line to the tenant's log store itself**, with the
ingest token, to the workload partition. It sends nothing else there: its
log stays in the journal. Without the endpoint and the token it counts into
its journal only, which is what it does on a desk.

**The dashboard is Outside Services**, declared in the tenant's Terraform
beside the stand's. It reads the device partition for the Pi and the
workload partition for nowplayd.

## Consequences

- **How much is asked of whom can be seen**, by the minute, and a 429 from
  any of them is a number on a dashboard and not a line in a journal.
- **nowplayd holds a second way out of the tenant**, to the log store, and
  uses a token it was always given. A store that cannot be reached is said
  once in the journal and changes nothing else.
- **The numbers are a minute coarse.** Three calls in one second and three
  spread over a minute look the same. MusicBrainz's rule is by the second,
  and this cannot show it being kept; only that the minute was small.
- **A call is counted once per request sent.** A redirect followed is two.
- **The recognizer's count is of clips, not of requests.** What its
  library sends for one clip is not seen from here.
- **Spotify's numbers are empty** while its driver is off
  ([CHG-0011](/docs/changes/2026/0011-spotify-turned-off/)).
- **A new client for an outside service has to be made by the counter**, or
  it is not counted, and nothing will say so.
- **The allowances on the dashboard are a note, written by hand** from what
  each service published. They go out of date without anything changing
  here.

## What would settle it

Tested: the counter's sums, a client that counts what it asks through a
local server, whose a host is, the line as the store wants it posted, and a
recognizer counted under the service it names.

Not yet shown:

- [ ] A line from `nowplayd` kept by the log store: the token, the store's
  certificate against the site's CA, and the way there from the workload.
- [ ] A line from the agent Pi arriving with its numbers as fields.
- [ ] The dashboard reading both partitions in one panel.
- [ ] A side of a record: the recognizer's calls a minute, and Discogs'.
- [ ] Spotify's driver on, and its count.
