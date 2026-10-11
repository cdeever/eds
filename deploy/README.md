# deploy

`palette`, `lightd`, `nowplayd` and the `console` as containers on the eds
`services` workload, under systemd. This is the tenant guide's *Deploy Your App* pattern: images built
here and copied over SSH, settings from the tenant's `kit.env`, no registry
and no operator.

```bash
make deploy     # first time, or onto a replaced workload: images, settings, units, start
make ship       # a new version of any service: build, copy, restart
make logs       # follow all four
```

Needs a container tool, SSH to `tenant@services.eds.mobile.deevnet.net` with a
key in the tenant's `ssh_keys`, and `infra/deevnet-tenant-eds/.backend.env`
(the state store's credentials, where the settings are read from).

**No container tool on this machine?** Add `WHERE=builder`:

```bash
make deploy WHERE=builder
```

The images are then built on the Builder, from a copy of `services/`, and
loaded onto the workload from there. Settings and units still go from this
machine: nothing secret is sent to the Builder. `CONTAINER=docker` uses Docker
in place of Podman where that is what there is.

| On the workload | |
|---|---|
| `eds-palette.service` | `localhost/eds-palette`, port 8731 |
| `eds-lightd.service` | `localhost/eds-lightd`, port 8732; starts after palette |
| `eds-nowplayd.service` | `localhost/eds-nowplayd`; no port, it only dials out |
| `/opt/eds/kit.env` | secret, 0600: lightd's broker login and the tenant's tokens |
| `/opt/eds/lightd.env` | lightd's own settings; overrides `kit.env` |
| `/opt/eds/nowplayd.env` | nowplayd's own settings |
| `/opt/eds/nowplayd-broker.env` | secret, 0600: nowplayd's broker login. `kit.env` carries one login, lightd's, so nowplayd's is generated beside it |
| `/opt/eds/deevnet-root-ca.pem` | the Deevnet Root CA, which the broker's certificate is checked against |

Both run with host networking, and the workload's firewall admits only SSH, so
neither port is reachable from outside the VM: lightd reaches palette on
localhost. To post a cover from your computer, tunnel:

```bash
ssh -L 8732:localhost:8732 tenant@services.eds.mobile.deevnet.net
curl --data-binary @jacket.jpg -H 'Content-Type: image/jpeg' http://localhost:8732/v1/cover
```

A replaced workload comes back empty; `make deploy` puts everything back.

## The console is the one thing reachable

The console serves a page at `https://console.eds.mobile.deevnet.net/`; port
80 only redirects there. `make units` opens `http` and `https` in the
workload's own firewall, which otherwise admits SSH and nothing else. The substrate lets the site's trusted
and management networks reach a workload; the guest and IoT networks, and
anything outside, cannot. The page only looks
([ADR-0018](../docs/content/docs/architecture/decisions/0018-a-console-of-eds-own.md)).

It runs as a user who is not root, with every capability dropped but the one
that lets it bind a low port, on a read-only filesystem.

**Its key and certificate come from the tenant's state.** `make tls-files`
writes them to `tls/`, which is ignored, and `make units` installs them on the
workload owned by the container's user and readable by no one else. The
certificate is the site's Tenant Service CA's (Deevnet ADR-0037) and lasts a
year. To renew: `make -C ../infra/deevnet-tenant-eds plan` shows it from sixty
days before; apply, then `make units`.

## What follows what

```
agent Pi ──▶ eds/nowplaying/source/… ──▶ nowplayd ──▶ eds/nowplaying/current/art ──▶ lightd ──▶ the stand
```

`nowplayd` decides what is playing and publishes its cover; `lightd` lights the
stand from that cover (`LIGHTD_COVER_TOPIC` in `lightd.env`). Neither calls the
other: they meet at the broker.

Everything restarts together on a `make ship`, and both services were written
for it:

- `lightd` is handed the retained cover the moment it subscribes, which is
  before `palette` is listening. It tries again for up to half a minute rather
  than drop a cover that will not be sent twice.
- `nowplayd` may be handed a source's state before the cover that state names.
  It waits two seconds for the cover rather than look one up.

To see what a cover produced: `make logs`, and `lightd`'s `published scene`
line carries each colour and its share of the strip.
