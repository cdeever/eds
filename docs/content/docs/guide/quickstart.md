---
title: "Running the Vertical"
weight: 1
---

# Running the Vertical

The whole chain on one machine, with no deevnet infrastructure.

## 1. Start the palette extractor

```bash
cd services/palette
make venv
make serve PORT=8731
```

## 2. Start a broker and lightd

```bash
cd services/lightd
make broker      # mosquitto in podman on tcp://127.0.0.1:21883
make run
```

`make run` builds `lightd` and points it at the development broker. It expects
`palette` on `:8731`.

## 3. Watch the topic tree

In another terminal:

```bash
cd services/lightd
make broker-watch      # tail every eds/# topic
```

## 4. Send a cover

```bash
curl -F image=@jacket.jpg http://localhost:8732/v1/cover
```

A scene should appear on `eds/lightstand/lp-stand-01/scene`. Because gamma is
applied on the device rather than on the wire, the colours you see here are the
colours actually intended.

To exercise a specific effect without hunting for cover art that produces it:

```bash
curl -X POST http://localhost:8732/v1/scene \
  -d '{"effect":"sweep","brightness":0.8,"palette":[
        {"rgb":[255,0,0],"weight":0.6},{"rgb":[0,0,255],"weight":0.4}]}'
```

## 5. Point a stand at it

Provision the stand with the broker URI set to this machine's LAN address and
port `21883` — not `localhost`, which on the ESP32 means the ESP32.

```bash
cd firmware/lp-stand
make wifi-config PORT=/dev/cu.usbserial-0001
```

The development broker allows anonymous connections, so leave the broker
username and password blank. See [Provisioning a Stand](../provisioning/).

Because the scene topic is retained, a stand that connects *after* the scene was
published still receives it. That is the normal case, not an edge one.
