// eds-ota sends a firmware build to one device through the broker (ADR-0035).
//
//	eds-ota -device lightstand/lp-stand-01 -image lp-stand.bin
//	eds-ota -device beacon/turntable-power-01 < power-beacon.bin
//
// It runs on the workload, which reaches the broker as lightd does, with the
// broker account `ota`: MQTT_HOST, MQTT_PORT and DEEVNET_TENANT from kit.env,
// OTA_MQTT_USERNAME and OTA_MQTT_PASSWORD from ota-broker.env, and
// MQTT_CA_FILE. `make ota` in a firmware directory runs it there. OTA_MQTT_URL
// overrides the broker, for a dev one.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/cdeever/eds/tools/ota/internal/image"
	"github.com/cdeever/eds/tools/ota/internal/link"
	"github.com/cdeever/eds/tools/ota/internal/send"
)

// projects says which firmware each kind of device runs. A build for another
// is not offered.
var projects = map[string]string{
	"lightstand": "lp-stand",
	"screen":     "cover-screen",
	"beacon":     "power-beacon",
}

func main() {
	device := flag.String("device", "", "kind/id of the device: lightstand/lp-stand-01, screen/cover-screen-01, beacon/turntable-power-01")
	path := flag.String("image", "-", "the build's .bin; - reads it from stdin")
	flag.Parse()

	if err := run(*device, *path); err != nil {
		fmt.Fprintln(os.Stderr, "eds-ota:", err)
		os.Exit(1)
	}
}

func run(device, path string) error {
	kind, id, ok := strings.Cut(device, "/")
	if !ok || id == "" || projects[kind] == "" {
		return fmt.Errorf("-device %q: want lightstand/<id>, screen/<id> or beacon/<id>", device)
	}

	img, err := read(path)
	if err != nil {
		return err
	}
	desc, err := image.Describe(img)
	if err != nil {
		return err
	}
	if desc.Project != projects[kind] {
		return fmt.Errorf("the image is %s's, and a %s runs %s: not sent", desc.Project, kind, projects[kind])
	}

	prefix := env("DEEVNET_TENANT", "eds")
	topics := send.TopicsFor(prefix, kind, id)
	url := os.Getenv("OTA_MQTT_URL")
	if url == "" {
		host := os.Getenv("MQTT_HOST")
		if host == "" {
			return errors.New("no broker: MQTT_HOST (kit.env) or OTA_MQTT_URL")
		}
		url = "tls://" + host + ":" + env("MQTT_PORT", "8883")
	}

	conn, err := link.Dial(link.Config{
		URL:      url,
		CAFile:   os.Getenv("MQTT_CA_FILE"),
		Username: os.Getenv("OTA_MQTT_USERNAME"),
		Password: os.Getenv("OTA_MQTT_PASSWORD"),
		ClientID: "eds-ota-" + id,
	}, topics.State)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("offering %s %s (%s, %d bytes) to %s\n", desc.Project, desc.Version, desc.ELF, len(img), device)
	shown := -10 // so that 0% is shown: -1/10 is 0 in Go
	res, err := send.Send(ctx, conn, topics, img, desc, send.DefaultWaits, func(next, size int) {
		if pct := next * 100 / size; pct/10 != shown/10 {
			shown = pct
			fmt.Printf("  %3d%%\n", pct)
		}
	})
	if err != nil {
		return err
	}
	switch {
	case res.Kept:
		fmt.Printf("kept: %s runs %s (%s), %s after the offer\n", device, res.Running.Version, res.Running.ELF, res.Duration.Round(time.Second))
		return nil
	default:
		return fmt.Errorf("undone: %s went back to %s (%s); the new build did not reach the broker", device, res.Running.Version, res.Running.ELF)
	}
}

func read(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
