---
title: "CHG-0008: The Stand Reports Its Colors"
weight: 8
---

# CHG-0008: The Stand Reports Its Colors

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | `lp-stand-01`, over USB from the development Mac |
| **How** | `make -C firmware/lp-stand build flash PORT=/dev/cu.usbserial-0001` |
| **Risk** | Low. Most likely to go wrong: a flash that also erased the stand's credentials, which would leave it off the network until provisioned again |
| **Decisions** | [ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/) |
| **Related** | [CHG-0001](/docs/changes/2026/0001-lp-stand-joins-its-tenant/) |

---

## Summary

When the stand applies a scene it reports the effect, how many colors and the
brightness. It does not say which colors. The firmware that does has been on
`main` since it was written for a dashboard panel that was not kept, and has
never been on the stand.

After this change `scene.applied` also carries the speed and the palette, as
`#c90027 65%, #ac0030 13%`, the form `lightd` logs it in. The log store then
holds what each stand was sent, in the stand's own words.

## Goal

- The stand runs firmware built from this commit.
- It rejoins the network and the broker without being provisioned again.
- Its next `scene.applied` event carries `speed` and `palette`.
- It still applies scenes.

## Scope

**In scope:** the application image on `lp-stand-01`.

**Out of scope:** its credentials, which are in their own partition and are
not written by a flash; the effects; the LEDs, which are still not attached.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The flash erases the credentials partition | the stand, off the network | `idf.py flash` writes the bootloader, the partition table and the application, each at its own offset; `creds` is none of them. If it happens, `make provision` puts them back |
| The larger event does not fit | every scene would go unreported | host tests hold the largest scene, eight colors, to fitting in one event |
| The stand is dark while it restarts | the room | it has no LEDs attached |

## Procedure

```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
make -C firmware/lp-stand test
make -C firmware/lp-stand build flash PORT=/dev/cu.usbserial-0001
```

## Verification

- `make -C firmware/lp-stand logs` shows `system.boot`, then the network and
  broker events, then `scene.applied` with a `palette`.
- A new album in iTunes produces another, with that cover's colors.

## Undo

Flash the commit before the palette was added. The credentials are untouched
either way.

## Outcome

| When (UTC) | What happened |
|---|---|
| 19:06 | Host tests passed. Built and flashed over USB. The flash wrote the OTA data, the application and one small block before it; the `creds` partition was not among them. |
| 19:06:27 | The stand started (`reason=power`), joined `DVNTM-IOT` on the first attempt and reached the broker 3.1 s after boot, with nothing provisioned again. |
| 19:06:27 | It applied the retained scene and reported it: `effect=breathe colors=3 brightness=0.8 speed=0.4 palette=#2a2a81 80%, #ec1800 16%, #f0da85 4%`. |

All of the goal's checks hold. The colors are the ones `lightd` sent for the
album loaded in iTunes at the time.

### Departures from the plan

- The firmware names itself `c9e3286-dirty`. It was built with this record
  written and not yet committed, so the tree was dirty by one documentation
  file. The code is the commit's.
- The same deployment window put a playing-time counter on the console
  (below), which is not this change and shares its pull request.

## Follow-ups

- [ ] The strip is still not attached.
- [ ] `breathe` lights the dominant color alone. With the palette now in
  each event, the other colors are visibly unused by the default effect.
