package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

// UtteranceFilter is every device's requests: a WAV, never retained
// (ADR-0042).
func UtteranceFilter(prefix string) string { return prefix + "/voice/+/utterance" }

// WokenFilter is every device saying it has just heard its wake word
// (ADR-0044): the music is turned down while it listens.
func WokenFilter(prefix string) string { return prefix + "/voice/+/woken" }

// HeardTopic is where a device is told what was made of its request.
func HeardTopic(prefix, device string) string { return prefix + "/voice/" + device + "/heard" }

// StatusTopic is voiced's own presence.
func StatusTopic(prefix string) string { return prefix + "/voiced/status" }

// Heard is what was made of a request, said back to the device that sent it.
// Understood is false when nothing was asked for, and Reason says why.
type Heard struct {
	V          int    `json:"v"`
	Text       string `json:"text"`
	Understood bool   `json:"understood"`
	Artist     string `json:"artist,omitempty"`
	Album      string `json:"album,omitempty"`
	Title      string `json:"title,omitempty"` // a song's (ADR-0055)
	Reason     string `json:"reason,omitempty"`
}

// Words are what a recording was written down as, and whether scribe is
// sure of them.
type Words struct {
	Text string
	// Confident is false when scribe's model thinks nothing was said, or is
	// not sure of what. Words it is not sure of are never acted on: given
	// what it cannot make out, the model can write its prompt back, and a
	// prompt that was a request once played the Beatles for nobody
	// (INC-0001).
	Confident bool
}

// Transcriber writes a recording down.
type Transcriber interface {
	Transcribe(ctx context.Context, wav []byte) (Words, error)
}

// Counter knows an artist's albums in order (ADR-0041).
type Counter interface {
	Discography(ctx context.Context, artist string) (resolve.Discography, error)
}

// Titler knows the other names an album's releases have gone by (ADR-0045).
// A Counter that is not one names no others.
type Titler interface {
	Titles(ctx context.Context, album resolve.StudioAlbum) ([]string, error)
}

// maxOtherTitles is how many of an album's other names are tried before the
// artist's next album.
const maxOtherTitles = 3

// Listener makes each request heard into a command.
type Listener struct {
	Prefix string
	Scribe Transcriber
	Albums Counter
	// Library is the owner's music: eds-inventory (ADR-0047) or the library
	// index (ADR-0046). Nil, or nothing there yet, and albums are guessed as
	// they were (ADR-0045).
	Library Library
	Publish func(topic string, payload []byte, retained bool) error
	Logger  *slog.Logger

	// Timeout bounds one request, from its words to its album: a request
	// answered a minute late is not what anyone meant. Zero is 30 seconds.
	Timeout time.Duration
}

// Handle takes one message from the utterance and woken filters.
func (l *Listener) Handle(msg bus.Message) {
	if device, ok := deviceSaid(l.Prefix, msg.Topic, "/woken"); ok {
		if !msg.Retained {
			l.command(contract.Duck, device)
		}
		return
	}
	device, ok := deviceOf(l.Prefix, msg.Topic)
	if !ok {
		return
	}
	if msg.Retained {
		// Someone else's mistake, and played again on every reconnect.
		l.logger().Warn("dropped a retained request; a request is never retained", "device", device)
		return
	}
	timeout := l.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	heard := l.hear(ctx, device, msg.Payload)
	payload, _ := json.Marshal(heard)
	if err := l.Publish(HeardTopic(l.Prefix, device), payload, false); err != nil {
		l.logger().Warn("could not say what was heard", "device", device, "error", err)
	}
	// The request is answered: the music comes back, whatever was made of
	// it. After an album asked for, the driver has already brought it back.
	l.command(contract.Unduck, device)
}

// command sends one of the commands that are not a request's: the duck and
// its end (ADR-0044).
func (l *Listener) command(name, device string) {
	c, err := contract.EncodeCommand(contract.Command{Command: name, From: "voiced"})
	if err == nil {
		err = l.Publish(contract.CommandTopic(l.Prefix), c, false)
	}
	if err != nil {
		l.logger().Warn("could not send "+name, "device", device, "error", err)
	}
}

func (l *Listener) hear(ctx context.Context, device string, wav []byte) Heard {
	started := time.Now()
	words, err := l.Scribe.Transcribe(ctx, wav)
	if err != nil {
		l.logger().Warn("a request could not be written down", "device", device, "bytes", len(wav), "error", err)
		return Heard{V: contract.Version, Reason: "it could not be written down"}
	}
	text := words.Text
	heard := Heard{V: contract.Version, Text: text}
	log := l.logger().With("device", device, "text", text, "written_ms", time.Since(started).Milliseconds())
	if !words.Confident {
		log.Info("not heard clearly")
		heard.Reason = "not heard clearly"
		return heard
	}

	owner := l.owner(log)
	r, err := Parse(text)
	if err != nil && owner != nil {
		r, err = ParseOwned(text)
	}
	if err != nil && owner != nil {
		r, err = ParseName(text)
	}
	if err != nil {
		log.Info("not a request")
		heard.Reason = "not a request for an album"
		return heard
	}
	switch r.Kind {
	case AnArtist:
		return l.playArtist(ctx, log, owner, heard, r.Artist, started)
	case ASong:
		return l.playTrack(ctx, log, owner, heard, r.Artist, r.Title, started)
	case AName:
		return l.playName(ctx, log, owner, heard, r.Title, started)
	}
	artist, album, instead, err := Choose(ctx, l.Albums, owner, r)
	if errors.Is(err, ErrNotOwned) && owner != nil && r.Album != "" {
		// "Play X by Y": not an album of Y's the owner has, so perhaps a
		// song of theirs (ADR-0055). Only for an artist the owner has.
		if a, _, aerr := ownedArtist(ctx, l.Albums, owner, r.Artist); aerr == nil && a != nil {
			return l.playTrack(ctx, log, owner, heard, a.Name, r.Album, started)
		}
	}
	if err != nil && owner != nil && !errors.Is(err, ErrNotOwned) && ctx.Err() == nil {
		// Asked, and not answered: the album is guessed at, as it is with no
		// library at all, and that is said.
		log.Warn("the owner's music could not be asked; the album is guessed at", "error", err)
		artist, album, instead, err = Album(ctx, l.Albums, r)
	}
	if err != nil {
		log.Info("no album for the request", "artist", r.Artist, "nth", r.Nth, "error", err)
		heard.Reason = "no such album: " + reason(err)
		return heard
	}

	return l.playAlbum(log, heard, artist, album, instead, started)
}

// send asks for what was chosen to play.
func (l *Listener) send(c contract.Command) error {
	c.From = "voiced"
	payload, err := contract.EncodeCommand(c)
	if err != nil {
		return err
	}
	return l.Publish(contract.CommandTopic(l.Prefix), payload, false)
}

func (l *Listener) playAlbum(log *slog.Logger, heard Heard, artist, album string, instead []string, started time.Time) Heard {
	if err := l.send(contract.Command{Command: contract.PlayAlbum, Artist: artist, Album: album, Instead: instead}); err != nil {
		log.Warn("could not ask for the album", "artist", artist, "album", album, "error", err)
		heard.Reason = "the album could not be asked for"
		return heard
	}
	log.Info("asked for an album", "artist", artist, "album", album, "total_ms", time.Since(started).Milliseconds())
	heard.Understood, heard.Artist, heard.Album = true, artist, album
	return heard
}

// artistFor is the owner's spelling of an artist heard, or the name as
// heard with no library, or one that does not answer. ok is false when the
// library answered and has nobody by that name.
func (l *Listener) artistFor(ctx context.Context, log *slog.Logger, owner Owner, heard string) (string, bool) {
	if owner == nil {
		return heard, true
	}
	a, _, err := ownedArtist(ctx, l.Albums, owner, heard)
	if err != nil {
		log.Warn("the owner's music could not be asked; the artist is asked for as heard", "error", err)
		return heard, true
	}
	if a == nil {
		return "", false
	}
	return a.Name, true
}

// playArtist asks for all of an artist's songs, shuffled (ADR-0055).
func (l *Listener) playArtist(ctx context.Context, log *slog.Logger, owner Owner, heard Heard, asked string, started time.Time) Heard {
	artist, ok := l.artistFor(ctx, log, owner, asked)
	if !ok {
		log.Info("no such artist", "artist", asked)
		heard.Reason = "no such artist: nothing by " + asked
		return heard
	}
	if err := l.send(contract.Command{Command: contract.PlayArtist, Artist: artist}); err != nil {
		log.Warn("could not ask for the artist", "artist", artist, "error", err)
		heard.Reason = "the artist could not be asked for"
		return heard
	}
	log.Info("asked for an artist", "artist", artist, "total_ms", time.Since(started).Milliseconds())
	heard.Understood, heard.Artist = true, artist
	return heard
}

// playTrack asks for a song by its artist and title (ADR-0055). The title
// is as said: the library knows no songs yet, and the player is the check.
func (l *Listener) playTrack(ctx context.Context, log *slog.Logger, owner Owner, heard Heard, asked, title string, started time.Time) Heard {
	artist, ok := l.artistFor(ctx, log, owner, asked)
	if !ok {
		log.Info("no such artist", "artist", asked)
		heard.Reason = "no such song: nothing by " + asked
		return heard
	}
	if err := l.send(contract.Command{Command: contract.PlayTrack, Artist: artist, Title: title}); err != nil {
		log.Warn("could not ask for the song", "artist", artist, "title", title, "error", err)
		heard.Reason = "the song could not be asked for"
		return heard
	}
	log.Info("asked for a song", "artist", artist, "title", title, "total_ms", time.Since(started).Milliseconds())
	heard.Understood, heard.Artist, heard.Title = true, artist, title
	return heard
}

// playName asks for a name said alone: the artist, if the owner has one by
// it, else the album, if one artist has an album by it (ADR-0055). Two or
// more are said back, so the request can be said again with one.
func (l *Listener) playName(ctx context.Context, log *slog.Logger, owner Owner, heard Heard, name string, started time.Time) Heard {
	a, _, err := owner.FindArtist(ctx, name)
	if err != nil {
		log.Warn("the owner's music could not be asked about a name alone", "name", name, "error", err)
		heard.Reason = "the library could not be asked"
		return heard
	}
	if a != nil {
		return l.playArtist(ctx, log, owner, heard, a.Name, started)
	}
	albums, err := owner.FindAlbums(ctx, name)
	if err != nil {
		log.Warn("the owner's music could not be asked about a name alone", "name", name, "error", err)
		heard.Reason = "the library could not be asked"
		return heard
	}
	switch len(albums) {
	case 0:
		log.Info("no artist or album by that name", "name", name)
		heard.Reason = "no artist or album called " + name
		return heard
	case 1:
		return l.playAlbum(log, heard, albums[0].Artist.Name, albums[0].Title, nil, started)
	}
	var by []string
	for _, al := range albums {
		by = append(by, al.Artist.Name)
	}
	log.Info("an album more than one artist has", "name", name, "artists", by)
	heard.Reason = name + " is by more than one: " + strings.Join(by, ", ")
	return heard
}

// Album names the album a request asks for: as said, or counted among the
// artist's studio albums. An artist heard without an apostrophe ("Van
// Halens") is asked for again without its s when no artist has the name.
//
// For an album asked for by its place, instead are the artist's next studio
// albums after it, up to contract.MaxInstead, to play if the library has not
// got it (ADR-0045): the later ones for the first or the Nth, the earlier
// ones for the latest.
func Album(ctx context.Context, albums Counter, r Request) (artist, album string, instead []string, err error) {
	if r.Album != "" {
		return r.Artist, r.Album, nil, nil
	}
	d, err := albums.Discography(ctx, r.Artist)
	if errors.Is(err, resolve.ErrNotFound) {
		if bare, ok := strings.CutSuffix(r.Artist, "s"); ok && strings.TrimSpace(bare) != "" {
			d, err = albums.Discography(ctx, bare)
		}
	}
	if err != nil {
		return "", "", nil, err
	}
	a, err := d.Nth(r.Nth)
	if err != nil {
		return "", "", nil, err
	}
	instead = otherTitles(ctx, albums, d, a)
	step := 1
	if r.Nth < 0 {
		step = -1
	}
	for n, next := r.Nth+step, 0; len(instead) < contract.MaxInstead && n != 0 && next < 3; n, next = n+step, next+1 {
		next, err := d.Nth(n)
		if err != nil {
			break
		}
		instead = append(instead, next.Title)
	}
	return d.Artist, a.Title, instead, nil
}

// otherTitles are the names album's releases have gone by that are safe to
// ask the library for: not the artist's own name, and not the beginning of
// another of the artist's albums or begun by one, since the player matches a
// title by its start - a Genesis reissue called "Genesis" would play their
// 1983 album. A lookup that fails names none: the album and the next ones
// are still tried.
func otherTitles(ctx context.Context, albums Counter, d resolve.Discography, album resolve.StudioAlbum) []string {
	titler, ok := albums.(Titler)
	if !ok {
		return nil
	}
	titles, err := titler.Titles(ctx, album)
	if err != nil {
		return nil
	}
	var safe []string
	for _, t := range titles {
		name := resolve.Simplify(t)
		if len(name) < 4 || name == resolve.Simplify(d.Artist) {
			continue
		}
		clash := false
		for _, other := range d.Albums {
			o := resolve.Simplify(other.Title)
			if other.ID != album.ID && (strings.HasPrefix(o, name) || strings.HasPrefix(name, o)) {
				clash = true
				break
			}
		}
		if !clash {
			safe = append(safe, t)
		}
		if len(safe) == maxOtherTitles {
			break
		}
	}
	return safe
}

func reason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 && errors.Is(err, resolve.ErrNotFound) {
		return msg[i+2:]
	}
	return msg
}

func deviceOf(prefix, topic string) (string, bool) {
	return deviceSaid(prefix, topic, "/utterance")
}

// deviceSaid is the device a voice/<device><suffix> topic is from.
func deviceSaid(prefix, topic, suffix string) (string, bool) {
	rest, ok := strings.CutPrefix(topic, prefix+"/voice/")
	if !ok {
		return "", false
	}
	device, ok := strings.CutSuffix(rest, suffix)
	if !ok || device == "" || strings.Contains(device, "/") {
		return "", false
	}
	return device, true
}

func (l *Listener) logger() *slog.Logger {
	if l.Logger != nil {
		return l.Logger
	}
	return slog.Default()
}

// Scribe asks scribe to write a recording down (ADR-0042).
type Scribe struct {
	URL  string // http://127.0.0.1:8733; lightd has 8732, palette 8731
	HTTP *http.Client
}

// Transcribe sends a WAV and returns its words. Words scribe does not say
// it is sure of are not confident, so a scribe from before it said so plays
// nothing.
func (s Scribe) Transcribe(ctx context.Context, wav []byte) (Words, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.URL, "/")+"/v1/transcribe", bytes.NewReader(wav))
	if err != nil {
		return Words{}, err
	}
	req.Header.Set("Content-Type", "audio/wav")
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Words{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Words{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Words{}, fmt.Errorf("scribe answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var t struct {
		V         int    `json:"v"`
		Text      string `json:"text"`
		Confident *bool  `json:"confident"`
	}
	if err := json.Unmarshal(body, &t); err != nil {
		return Words{}, fmt.Errorf("scribe: %w", err)
	}
	if t.V != 1 {
		return Words{}, fmt.Errorf("scribe speaks version %d, this program 1", t.V)
	}
	return Words{Text: t.Text, Confident: t.Confident != nil && *t.Confident}, nil
}

// owner is the owner's music, or nil: none configured, none copied yet, or
// an index that cannot be read, which is said, since the requests are then
// guessed at (ADR-0046). The first index copied was unreadable to voiced's
// container, and nothing said so (2026-10-24).
func (l *Listener) owner(log *slog.Logger) Owner {
	if l.Library == nil {
		return nil
	}
	o, err := l.Library.Owner()
	if err != nil {
		log.Warn("the library index cannot be read; albums are guessed at", "error", err)
	}
	return o
}
