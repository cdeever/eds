---
title: "EdS"
type: docs
---

# EdS

<p class="subtitle"><em>An intelligent music experience platform that listens, identifies, plays, and responds to music through sound, light, and personality.</em></p>

EdS is built as a monorepo. Each project is a distinct piece of the system, and
each is developable on its own.

## The first vertical

A stand that props up the LP jacket while vinyl spins, lit by an RGB strip that
reacts to the record's cover art.

```
cover image ──▶ lightd ──▶ palette ──▶ lightd ──▶ mqtt01 ──▶ LP stand
                (Go)       (Python)               (VLAN 35)   (VLAN 30)
```

It was chosen first because it touches every layer — firmware, a tenant-hosted
service, the MQTT boundary between house and datacentre, and the deevnet
substrate — while depending on none of the still-unscoped parts: listening,
identification, playback, personality.

## Where to start

<div class="section-cards">
  <a class="section-card" href="docs/architecture/">
    <h3>Architecture</h3>
    <p>How the pieces fit, and which layer is allowed to decide what.</p>
  </a>
  <a class="section-card" href="docs/guide/">
    <h3>User Guide</h3>
    <p>Setting up a stand and running the services.</p>
  </a>
  <a class="section-card" href="docs/develop/">
    <h3>Developer Guide</h3>
    <p>Building, testing and bringing up each project.</p>
  </a>
  <a class="section-card" href="docs/reference/">
    <h3>Reference</h3>
    <p>The contracts: HTTP APIs, the scene descriptor, MQTT topics.</p>
  </a>
  <a class="section-card" href="docs/architecture/decisions/">
    <h3>Decisions</h3>
    <p>Why it is built this way, and what was turned down. Read before reopening a question.</p>
  </a>
  <a class="section-card" href="docs/changes/">
    <h3>Changes and Incidents</h3>
    <p>What was changed on something running, and what happened.</p>
  </a>
</div>
