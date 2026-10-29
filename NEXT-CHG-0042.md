# CHG-0042: where it stopped (2026-10-27)

Done: the Pi's request was signed by the tenant's Terraform
(`deevnet_certificate.backend`, applied). The chain is checked: it chains to
the Deevnet Root and is for the Pi's key. Branch `chg-0042-backend-cert`,
committed in this worktree, not pushed.

Next, all on **home Wi-Fi** (the Pi can't be reached from dvntm).

## 1. Install the certificate on the Pi

```bash
scp ~/eds/deploy/backend/fullchain.pem \
    ~/eds/infra/deevnet-tenant-eds/deevnet-root-ca.pem eds@<pi>.local:
ssh eds@<pi>.local
sudo deevnet-kit cert install fullchain.pem deevnet-root-ca.pem
sudo systemctl restart eds-backend-setup
```

`cert install` checks everything before changing anything, and keeps the
card's own CA so you can go back.

## 2. Check it

On the Pi:

```bash
sudo deevnet-kit status                  # names mqtt.eds.deevnet.net
grep MQTT_HOST /etc/eds/lightd.env       # MQTT_HOST=mqtt.eds.deevnet.net
```

From the Mac: `make -C ~/eds/deploy/backend status`. Every check should say
ok, and the IPv6 warning should be gone.

## 3. Home DNS

On the home router, point these at the Pi's reserved IP:

- `mqtt`, `logs`, `grafana`, `console` . `eds.deevnet.net`
- `mqtt`, `logs`, `grafana`, `console` . `eds.home.deevnet.net`

Check: `https://console.eds.deevnet.net/` loads with no warning on the Mac.

## 4. Then

- Fill in CHG-0042's Outcome table (steps 5 and 6) and open the PR.
- Step 7, moving the devices, comes last, the beacon first. Its
  `--also mobile=terraform` reads the tenant's state, so run it on dvntm.
