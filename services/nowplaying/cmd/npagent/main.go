// Command npagent runs now-playing drivers.
//
// One driver exists so far, for iTunes and the Music app (ADR-0011). It pairs
// with the player the way a phone remote does and reports each change the
// player pushes.
//
//	npagent pair     advertise as a remote and wait for the code to be typed
//	npagent run      follow the player and publish to the bus
//	npagent watch    follow the player and print, publishing nothing
//	npagent forget   discard the stored pairing
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"log/slog"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/itunes"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "pair":
		err = pair(ctx)
	case "run":
		err = run(ctx)
	case "watch":
		err = watch(ctx)
	case "forget":
		err = forget()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "npagent:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: npagent <command>

  pair     advertise as a remote and wait for its code to be typed into iTunes
  run      follow the player and publish what it does to the bus
  watch    follow the player and print what it does, publishing nothing
  forget   discard the stored pairing

Settings, from the environment:
  NP_STATE_DIR    where the pairing is kept (default: the user config directory)
  NP_REMOTE_NAME  the name iTunes shows while pairing (default: EdS)
  NP_ITUNES_HOST  the player's address, to skip what pairing recorded
  NP_ITUNES_PORT  the player's port (default 3689)
  NP_PAIR_PORT    the port the player calls back to while pairing (default: any)
  NP_SOURCE_ID    what this deployment is called on the bus (default: itunes)
  NP_PLAYER       itunes or music (default: itunes)
  NP_MQTT_*, NP_TOPIC_PREFIX   the broker, as for nowplayd`)
}

// --- state -------------------------------------------------------------------

func stateDir() (string, error) {
	if dir := os.Getenv("NP_STATE_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "eds-npagent"), nil
}

func pairingPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "itunes-pairing.json"), nil
}

func loadPairing() (*dacp.Pairing, error) {
	path, err := pairingPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p dacp.Pairing
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &p, nil
}

// savePairing keeps the GUID private. It is the whole credential: anyone who
// holds it can control the player.
func savePairing(p *dacp.Pairing) (string, error) {
	path, err := pairingPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	return path, os.WriteFile(path, append(data, '\n'), 0o600)
}

func forget() error {
	path, err := pairingPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("forgot the pairing in", path)
	fmt.Println("iTunes still remembers this remote until it is removed there:")
	fmt.Println("  Edit > Preferences > Devices > \"Forget All Remotes\"")
	return nil
}

// --- pair --------------------------------------------------------------------

func pair(ctx context.Context) error {
	name := os.Getenv("NP_REMOTE_NAME")
	if name == "" {
		name = "EdS"
	}

	guid, err := dacp.NewGUID()
	if err != nil {
		return err
	}
	pin, err := dacp.NewPIN()
	if err != nil {
		return err
	}

	// The player calls back to this port once the code has been typed. Any
	// free port will do on a desk; a host with a firewall names one, so the
	// firewall can admit exactly that.
	listenOn := ":0"
	if raw := os.Getenv("NP_PAIR_PORT"); raw != "" {
		var fixed int
		if _, err := fmt.Sscan(raw, &fixed); err != nil || fixed < 1 || fixed > 65535 {
			return fmt.Errorf("NP_PAIR_PORT=%q is not a port", raw)
		}
		listenOn = fmt.Sprintf(":%d", fixed)
	}
	listener, err := net.Listen("tcp4", listenOn)
	if err != nil {
		return err
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	done := make(chan dacp.Paired, 1)
	calls := make(chan string, 16)
	handler := dacp.PairHandler(guid, pin, name, done)
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case calls <- fmt.Sprintf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr):
			default:
			}
			handler.ServeHTTP(w, r)
		}),
	}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	stopAdvert, advertDied, err := advertise(ctx, name, port, dacp.TXT(guid, name))
	if err != nil {
		return err
	}
	defer stopAdvert()

	fmt.Printf("Advertising a remote called %q on port %d.\n\n", name, port)
	fmt.Println("In iTunes on the PC:")
	fmt.Println("  1. Look for a small Remote button near the top left, beside the")
	fmt.Println("     media picker. It appears only while a remote is asking to pair.")
	fmt.Printf("  2. Click it, choose %q, and type this code:\n\n", name)
	fmt.Printf("         %s  %s  %s  %s\n\n", pin[0:1], pin[1:2], pin[2:3], pin[3:4])
	fmt.Println("Waiting up to five minutes. Ctrl-C to give up.")
	fmt.Println()

	timeout := time.After(5 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("nobody typed the code. If no Remote button appeared, iTunes never saw the advertisement: " +
				"check the PC's firewall admits Bonjour (UDP 5353) and that both machines are on the same network")
		case err := <-advertDied:
			// Without the advertisement the player never offers to pair, and
			// waiting out the five minutes would only hide why.
			return fmt.Errorf("the Bonjour advertisement stopped, so the player cannot see this remote: %w "+
				"(is the Bonjour daemon running? on Linux: systemctl status avahi-daemon)", err)
		case call := <-calls:
			fmt.Println("  the player called:", call)
		case paired := <-done:
			p := &dacp.Pairing{GUID: guid, Host: paired.Host, Port: dacp.DefaultPort, Name: name}
			path, err := savePairing(p)
			if err != nil {
				return err
			}
			fmt.Printf("\nPaired with the player at %s. Kept in %s\n", paired.Host, path)

			// Pairing is only half of it. Whether the player then lets this
			// remote in is the thing actually being tested.
			//
			// iTunes answers 503 for a few seconds after a pairing, while its
			// own dialog is still closing. The pairing is good and already
			// saved; this only waits for the player to be ready to say so.
			client := dacp.NewClient(p.Host, p.Port)
			var status dacp.Status
			for attempt := 1; ; attempt++ {
				check, cancel := context.WithTimeout(ctx, 10*time.Second)
				err = client.Login(check, p.GUID)
				if err == nil {
					status, err = client.Now(check)
				}
				cancel()
				if err == nil {
					break
				}
				if attempt == 8 || ctx.Err() != nil {
					fmt.Println("The pairing is saved, but the player has not let this remote in yet:", err)
					fmt.Println("Try `npagent watch` in a moment.")
					return nil
				}
				time.Sleep(3 * time.Second)
			}
			fmt.Println("Logged in. Right now:", describe(status))
			fmt.Println("\nNext: npagent watch")
			return nil
		}
	}
}

// advertise announces the remote over Bonjour until the returned function is
// called. It leans on the system's own tool rather than a library, so what the
// player sees is exactly what the system's Bonjour daemon sends.
//
// The channel reports the tool exiting by itself, which it does at once when
// there is no daemon to talk to.
func advertise(ctx context.Context, name string, port int, txt map[string]string) (func(), <-chan error, error) {
	// The instance name is not shown to anyone; the player lists DvNm.
	var raw [20]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, nil, err
	}
	instance := hex.EncodeToString(raw[:])

	keys := make([]string, 0, len(txt))
	for k := range txt {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		args := []string{"-R", instance, "_touch-remote._tcp", "local", fmt.Sprint(port)}
		for _, k := range keys {
			args = append(args, k+"="+txt[k])
		}
		cmd = exec.CommandContext(ctx, "dns-sd", args...)
	case "linux":
		args := []string{"-s", instance, "_touch-remote._tcp", fmt.Sprint(port)}
		for _, k := range keys {
			args = append(args, k+"="+txt[k])
		}
		cmd = exec.CommandContext(ctx, "avahi-publish", args...)
	default:
		return nil, nil, fmt.Errorf("advertising over Bonjour is not written for %s yet", runtime.GOOS)
	}

	output := &strings.Builder{}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("could not advertise over Bonjour: %w", err)
	}

	stopped := make(chan struct{})
	died := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		select {
		case <-stopped:
		default:
			said := strings.TrimSpace(output.String())
			if err == nil {
				err = errors.New("the advertising tool exited")
			}
			if said != "" {
				err = fmt.Errorf("%w: %s", err, said)
			}
			died <- err
		}
	}()

	return func() {
		close(stopped)
		_ = cmd.Process.Kill()
	}, died, nil
}

// --- run ---------------------------------------------------------------------

// player is the stored pairing with the environment's overrides applied.
func player() (*dacp.Pairing, error) {
	p, err := loadPairing()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("not paired yet: run `npagent pair` first")
		}
		return nil, err
	}
	if host := os.Getenv("NP_ITUNES_HOST"); host != "" {
		p.Host = host
	}
	if raw := os.Getenv("NP_ITUNES_PORT"); raw != "" {
		if _, err := fmt.Sscan(raw, &p.Port); err != nil || p.Port < 1 || p.Port > 65535 {
			return nil, fmt.Errorf("NP_ITUNES_PORT=%q is not a port", raw)
		}
	}
	return p, nil
}

// busSink puts what a driver sees on the bus.
type busSink struct {
	bus    *bus.Bus
	prefix string
	source string
	logger *slog.Logger

	mu     sync.Mutex
	online bool
}

// Online publishes the driver's presence. It is the driver's to say, not the
// connection's: a driver that cannot reach its player is offline however
// healthy its link to the broker is.
func (s *busSink) Online(online bool) {
	s.mu.Lock()
	s.online = online
	s.mu.Unlock()
	s.announce()
}

// announce says what is true now. It is also what runs after a reconnect,
// when the broker has just published "offline" on this driver's behalf.
func (s *busSink) announce() {
	s.mu.Lock()
	payload := contract.Offline
	if s.online {
		payload = contract.Online
	}
	s.mu.Unlock()

	if s.bus == nil {
		return
	}
	if err := s.bus.Publish(contract.SourceStatusTopic(s.prefix, s.source), []byte(payload), true); err != nil {
		s.logger.Warn("could not publish presence", "error", err)
	}
}

func (s *busSink) Publish(u itunes.Update) {
	// The cover first: whoever reads the state must find the image it names.
	if u.Cover != nil {
		if err := s.bus.Publish(contract.SourceArtTopic(s.prefix, s.source), u.Cover, true); err != nil {
			s.logger.Warn("could not publish the cover", "error", err)
			return
		}
	}
	payload, err := contract.Encode(u.State)
	if err != nil {
		s.logger.Warn("a state the contract refuses was not published", "error", err)
		return
	}
	if err := s.bus.Publish(contract.SourceStateTopic(s.prefix, s.source), payload, true); err != nil {
		s.logger.Warn("could not publish the state", "error", err)
		return
	}

	attrs := []any{"state", u.State.State, "cover_bytes", len(u.Cover)}
	if u.State.Track != nil {
		attrs = append(attrs, "artist", u.State.Track.Artist, "album", u.State.Track.Album, "title", u.State.Track.Title)
	}
	s.logger.Info("published", attrs...)
}

func run(ctx context.Context) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	p, err := player()
	if err != nil {
		return err
	}

	source := config.Str("NP_SOURCE_ID", "itunes")
	if !contract.ValidSourceID(source) {
		return fmt.Errorf("NP_SOURCE_ID=%q cannot name a source: lowercase letters, digits and hyphens", source)
	}
	kind := config.Str("NP_PLAYER", "itunes")
	if kind != "itunes" && kind != "music" {
		return fmt.Errorf("NP_PLAYER=%q: itunes or music", kind)
	}

	common, err := config.LoadCommon("eds-npagent-" + source)
	if err != nil {
		return err
	}

	sink := &busSink{prefix: common.Prefix, source: source, logger: logger}

	cfg := common.Broker
	// Presence is the driver's to announce, so nothing is announced for it
	// automatically; the will covers the case where it cannot speak.
	cfg.WillTopic = contract.SourceStatusTopic(common.Prefix, source)
	cfg.OnConnect = sink.announce

	client, err := bus.Connect(cfg, nil, nil)
	if err != nil {
		return err
	}
	defer client.Close()
	sink.bus = client

	logger.Info("connected to broker", "url", cfg.URL,
		"state", contract.SourceStateTopic(common.Prefix, source), "player", fmt.Sprintf("%s:%d", p.Host, p.Port))

	driver := &itunes.Driver{
		Source: source,
		Kind:   kind,
		GUID:   p.GUID,
		Player: dacp.NewClient(p.Host, p.Port),
		Sink:   sink,
		Logger: logger,
	}
	err = driver.Run(ctx)
	if errors.Is(err, context.Canceled) {
		logger.Info("shutting down")
		return nil
	}
	return err
}

// --- watch -------------------------------------------------------------------

func watch(ctx context.Context) error {
	p, err := player()
	if err != nil {
		return err
	}

	dir, err := stateDir()
	if err != nil {
		return err
	}
	fmt.Printf("Watching the player at %s:%d. Ctrl-C to stop.\n\n", p.Host, p.Port)

	backoff := time.Second
	for ctx.Err() == nil {
		err := follow(ctx, p, dir)
		if ctx.Err() != nil {
			return nil
		}

		var refused *dacp.StatusError
		if errors.As(err, &refused) && refused.Code == http.StatusForbidden {
			return fmt.Errorf("the player no longer knows this remote (%w): pair again", err)
		}
		fmt.Printf("%s  lost the player: %v; trying again in %s\n", stamp(), err, backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return nil
}

// follow logs in and prints every change until something goes wrong.
func follow(ctx context.Context, p *dacp.Pairing, dir string) error {
	client := dacp.NewClient(p.Host, p.Port)

	login, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := client.Login(login, p.GUID)
	cancel()
	if err != nil {
		return err
	}

	first, cancel := context.WithTimeout(ctx, 15*time.Second)
	status, err := client.Now(first)
	cancel()
	if err != nil {
		return err
	}

	var lastAlbum uint64
	var lastTrack string
	for {
		fmt.Printf("%s  %s\n", stamp(), describe(status))

		track := status.Artist + "\x00" + status.Album + "\x00" + status.Title
		if (status.Playing || status.Paused) && (status.AlbumID != lastAlbum || (status.AlbumID == 0 && track != lastTrack)) {
			started := time.Now()
			art, err := client.Artwork(ctx, 600)
			switch {
			case err != nil:
				fmt.Printf("%s    cover: could not be fetched: %v\n", stamp(), err)
			case art == nil:
				fmt.Printf("%s    cover: the player has none for this\n", stamp())
			default:
				path := filepath.Join(dir, "itunes-cover"+extension(art))
				_ = os.WriteFile(path, art, 0o600)
				fmt.Printf("%s    cover: %s, %d KB in %s -> %s\n", stamp(),
					http.DetectContentType(art), len(art)/1024, time.Since(started).Round(time.Millisecond), path)
			}
		}
		lastAlbum, lastTrack = status.AlbumID, track

		// This is the request that hangs until something changes.
		waited := time.Now()
		next, err := client.Wait(ctx, status.Revision)
		if err != nil {
			return fmt.Errorf("after waiting %s: %w", time.Since(waited).Round(time.Second), err)
		}
		if next.Revision == status.Revision {
			// The player answered without anything new. Not expected; noted
			// so it shows up, and slowed so it cannot spin.
			fmt.Printf("%s    (answered after %s with the same revision %d)\n", stamp(),
				time.Since(waited).Round(time.Millisecond), next.Revision)
			time.Sleep(time.Second)
		}
		status = next
	}
}

func describe(s dacp.Status) string {
	state := "stopped"
	switch {
	case s.Playing:
		state = "playing"
	case s.Paused:
		state = "paused "
	}
	if s.Title == "" && s.Artist == "" {
		return fmt.Sprintf("%s  (nothing loaded)  rev %d", state, s.Revision)
	}
	return fmt.Sprintf("%s  %s - %s  [%s]  %s / %s  rev %d", state, s.Artist, s.Title, s.Album,
		clock(s.PositionMS()), clock(s.DurationMS), s.Revision)
}

func clock(ms uint32) string {
	return fmt.Sprintf("%d:%02d", ms/60000, ms/1000%60)
}

func stamp() string { return time.Now().Format("15:04:05") }

func extension(image []byte) string {
	switch http.DetectContentType(image) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	}
	return ".img"
}
