// Package spotify is the driver for Spotify.
//
// Spotify has no local presence to pair with. The driver asks Spotify's
// servers what the account is playing, every few seconds, and reports that
// (ADR-0012). There is no push.
//
// Two rules of its own:
//
//   - Only named devices count. The account plays wherever its owner is - a
//     phone, a car - and the answer wanted is what is playing here.
//   - It never fetches a picture. Spotify is reported without its cover
//     (ADR-0016), so there is no request for one to make.
package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/spotifyauth"
)

// API is where Spotify's Web API is.
const API = "https://api.spotify.com"

// Tokens hands out access tokens. spotifyauth.Source is the real one.
type Tokens interface {
	Access(ctx context.Context) (string, error)
	Invalidate()
	Token() spotifyauth.Token
}

// Driver follows one Spotify account.
type Driver struct {
	Source string // the source id this driver reports as
	API    string
	HTTP   *http.Client
	Tokens Tokens
	Sink   driver.Sink
	Logger *slog.Logger

	// Devices are the names of the devices whose playback counts, as Spotify
	// shows them. Compared without regard to case.
	Devices []string

	// Interval is how often Spotify is asked. Ten seconds is what other
	// clients of this endpoint use; Spotify publishes no limit, only that
	// there is one.
	Interval time.Duration

	Now func() time.Time

	// after is time.After, replaceable so tests can see how long was waited.
	after func(time.Duration) <-chan time.Time
}

// player is the part of Spotify's answer the driver reads.
type player struct {
	Device struct {
		Name    string `json:"name"`
		Private bool   `json:"is_private_session"`
	} `json:"device"`
	IsPlaying  bool   `json:"is_playing"`
	Type       string `json:"currently_playing_type"`
	ProgressMS int64  `json:"progress_ms"`
	Item       *struct {
		Name       string `json:"name"`
		DurationMS int64  `json:"duration_ms"`
		Artists    []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"album"`
	} `json:"item"`
}

// errForbidden is Spotify refusing the question itself, not the token: the
// app's owner has no Premium subscription, or the account was never added to
// the app. Asking again in ten seconds changes neither.
type errForbidden struct{ why error }

func (e errForbidden) Error() string { return e.why.Error() }

// forbiddenWait is how long a refusal is left alone. Spotify says a change of
// subscription takes hours to be noticed, so nothing is lost by it.
const forbiddenWait = 5 * time.Minute

// errRateLimited carries how long Spotify asked to be left alone.
type errRateLimited struct{ wait time.Duration }

func (e errRateLimited) Error() string { return fmt.Sprintf("spotify: rate limited for %s", e.wait) }

// Run follows the account until ctx ends. It returns early only for a reason
// that waiting will not cure: no devices named, or an authorization that has
// ended and has to be given again by a person.
func (d *Driver) Run(ctx context.Context) error {
	if len(d.wanted()) == 0 {
		return errors.New("spotify: no devices are named, so no playback would count (NP_SPOTIFY_DEVICES)")
	}
	defer d.Sink.Online(false)

	var (
		last      *contract.State
		changedAt time.Time
		online    bool
		failures  int
		warned    time.Time
	)

	for {
		if d.now().Sub(warned) >= 24*time.Hour {
			d.warnIfExpiring()
			warned = d.now()
		}

		wait := d.interval()
		state, err := d.poll(ctx)
		var limited errRateLimited
		var forbidden errForbidden
		switch {
		case ctx.Err() != nil:
			return ctx.Err()

		case errors.Is(err, spotifyauth.ErrReauthorize):
			// Not retried: the same request would be refused the same way.
			return err

		case errors.As(err, &limited):
			// Being told to wait is not an outage. Say nothing has changed
			// and do as asked.
			d.logger().Warn("spotify asked to be left alone", "for", limited.wait)
			if limited.wait > wait {
				wait = limited.wait
			}

		case errors.As(err, &forbidden):
			// Offline at once, and not hurried: this is not the internet
			// having a moment.
			d.logger().Error("spotify refuses to say what is playing", "error", err, "asking_again_in", forbiddenWait)
			if online {
				online = false
				d.Sink.Online(false)
			}
			wait = forbiddenWait

		case err != nil:
			failures++
			d.logger().Warn("could not ask spotify", "error", err, "failures", failures)
			// One failed request is the internet. Several running is a
			// driver that does not know what is playing, and should say so.
			if failures == 3 && online {
				online = false
				d.Sink.Online(false)
			}

		default:
			failures = 0
			if !online {
				online = true
				d.Sink.Online(true)
			}
			if last == nil || last.State != state.State || key(last) != key(&state) {
				changedAt = d.now()
			}
			state.ChangedAt = changedAt
			if !contract.Same(last, &state) {
				d.Sink.Publish(driver.Update{State: state})
				kept := state
				last = &kept
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.sleep(wait):
		}
	}
}

// poll asks once. A refused access token is refreshed and the question asked
// again, once: tokens are refused for being stale far more often than for
// being wrong.
func (d *Driver) poll(ctx context.Context) (contract.State, error) {
	state, status, err := d.ask(ctx)
	if status == http.StatusUnauthorized {
		d.Tokens.Invalidate()
		state, status, err = d.ask(ctx)
		if status == http.StatusUnauthorized {
			return state, errors.New("spotify: refused a freshly issued access token")
		}
	}
	return state, err
}

func (d *Driver) ask(ctx context.Context) (contract.State, int, error) {
	stopped := contract.State{V: contract.Version, Source: d.Source, Player: "spotify", State: contract.Stopped}

	token, err := d.Tokens.Access(ctx)
	if err != nil {
		return stopped, 0, err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.api()+"/v1/me/player", nil)
	if err != nil {
		return stopped, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.HTTP.Do(req)
	if err != nil {
		return stopped, 0, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		// Nothing is playing anywhere on the account.
		return stopped, resp.StatusCode, nil
	case http.StatusUnauthorized:
		return stopped, resp.StatusCode, nil
	case http.StatusTooManyRequests:
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if seconds < 1 {
			seconds = 30
		}
		return stopped, resp.StatusCode, errRateLimited{time.Duration(seconds) * time.Second}
	case http.StatusForbidden:
		return stopped, resp.StatusCode, errForbidden{refusal(resp)}
	case http.StatusOK:
	default:
		return stopped, resp.StatusCode, refusal(resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return stopped, resp.StatusCode, err
	}
	var p player
	if err := json.Unmarshal(body, &p); err != nil {
		return stopped, resp.StatusCode, fmt.Errorf("spotify's answer could not be read: %w", err)
	}
	return d.toState(p), resp.StatusCode, nil
}

// toState applies the driver's rules. Everything that is not "a track, on a
// device that counts, in the open" is reported as stopped: not hidden, and
// not described.
func (d *Driver) toState(p player) contract.State {
	state := contract.State{V: contract.Version, Source: d.Source, Player: "spotify", State: contract.Stopped}

	switch {
	case p.Item == nil:
		return state
	case p.Type != "" && p.Type != "track":
		// An advertisement or a podcast episode: not an album, and not what
		// this is for.
		return state
	case p.Device.Private:
		// The owner asked Spotify not to share what they are listening to.
		// That it reaches this far is no reason to pass it on.
		return state
	case !d.counts(p.Device.Name):
		return state
	}

	if p.IsPlaying {
		state.State = contract.Playing
	} else {
		state.State = contract.Paused
	}

	names := make([]string, 0, len(p.Item.Artists))
	for _, a := range p.Item.Artists {
		names = append(names, a.Name)
	}
	state.Track = &contract.Track{
		Title:      p.Item.Name,
		Artist:     strings.Join(names, ", "),
		Album:      p.Item.Album.Name,
		DurationMS: p.Item.DurationMS,
	}
	if len(p.Item.Album.Artists) > 0 {
		state.Track.AlbumArtist = p.Item.Album.Artists[0].Name
	}
	position := p.ProgressMS
	state.PositionMS = &position
	return state
}

func (d *Driver) wanted() []string {
	var out []string
	for _, name := range d.Devices {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (d *Driver) counts(device string) bool {
	for _, name := range d.wanted() {
		if strings.EqualFold(name, strings.TrimSpace(device)) {
			return true
		}
	}
	return false
}

// warnIfExpiring says so in good time. Spotify ends an authorization six
// months after it was given whatever is done with it, and the first anyone
// would otherwise hear of it is the driver going quiet.
func (d *Driver) warnIfExpiring() {
	left := d.Tokens.Token().Expires().Sub(d.now())
	if left < 14*24*time.Hour {
		d.logger().Warn("the spotify authorization is ending; authorize again",
			"days_left", int(left.Hours()/24), "ends", d.Tokens.Token().Expires().Format("2006-01-02"))
	}
}

func key(s *contract.State) string {
	if s == nil || s.Track == nil {
		return ""
	}
	return s.Track.Key()
}

func (d *Driver) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Driver) sleep(wait time.Duration) <-chan time.Time {
	if d.after != nil {
		return d.after(wait)
	}
	return time.After(wait)
}

func (d *Driver) interval() time.Duration {
	if d.Interval > 0 {
		return d.Interval
	}
	return 10 * time.Second
}

func (d *Driver) api() string {
	if d.API != "" {
		return d.API
	}
	return API
}

func (d *Driver) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// refusal is an answer that was not the one asked for, with Spotify's own
// reason when it gave one. The reason is the only way to tell an account
// that is not Premium from an app the account was never added to: both are
// a bare 403.
func refusal(resp *http.Response) error {
	var said struct {
		Error struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if json.Unmarshal(body, &said) == nil && said.Error.Message != "" {
		if said.Error.Reason != "" {
			return fmt.Errorf("spotify answered %s: %s (%s)", resp.Status, said.Error.Message, said.Error.Reason)
		}
		return fmt.Errorf("spotify answered %s: %s", resp.Status, said.Error.Message)
	}
	if text := strings.TrimSpace(string(body)); text != "" && len(text) < 200 {
		return fmt.Errorf("spotify answered %s: %s", resp.Status, text)
	}
	return fmt.Errorf("spotify answered %s", resp.Status)
}

// Device is one of the account's devices, as Spotify names it.
type Device struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Active bool   `json:"is_active"`
}

// ListDevices asks which devices the account has open right now. Spotify
// lists only those: a device shows up while its app is running.
func ListDevices(ctx context.Context, client *http.Client, api, accessToken string) ([]Device, error) {
	if api == "" {
		api = API
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"/v1/me/player/devices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, refusal(resp)
	}
	var answer struct {
		Devices []Device `json:"devices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&answer); err != nil {
		return nil, err
	}
	return answer.Devices, nil
}
