# deploy

`palette` and `lightd` as containers on the eds `services` workload, under
systemd. This is the tenant guide's *Deploy Your App* pattern: images built
here and copied over SSH, settings from the tenant's `kit.env`, no registry
and no operator.

```bash
make deploy     # first time, or onto a replaced workload: images, kit.env, units, start
make ship       # a new version of either service: build, copy, restart
make logs       # follow both
```

Needs Podman, SSH to `tenant@services.eds.mobile.deevnet.net` with a key in
the tenant's `ssh_keys`, and `infra/deevnet-tenant-eds/.backend.env` (the state
store's credentials, where `kit.env` is read from).

| On the workload | |
|---|---|
| `eds-palette.service` | `localhost/eds-palette`, port 8731 |
| `eds-lightd.service` | `localhost/eds-lightd`, port 8732; starts after palette |
| `/opt/eds/kit.env` | secret, 0600: lightd's broker login and the tenant's tokens |
| `/opt/eds/lightd.env` | lightd's own settings; overrides `kit.env` |
| `/opt/eds/site-ca.pem` | the site CA the broker's certificate is checked against |

Both run with host networking, and the workload's firewall admits only SSH, so
neither port is reachable from outside the VM: lightd reaches palette on
localhost. To post a cover from your computer, tunnel:

```bash
ssh -L 8732:localhost:8732 tenant@services.eds.mobile.deevnet.net
curl --data-binary @jacket.jpg -H 'Content-Type: image/jpeg' http://localhost:8732/v1/cover
```

A replaced workload comes back empty; `make deploy` puts everything back.
