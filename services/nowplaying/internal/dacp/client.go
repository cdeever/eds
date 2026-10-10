package dacp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// DefaultPort is where iTunes and the Music app serve DACP.
const DefaultPort = 3689

// Play states, as the cmst "caps" item reports them.
const (
	capsStopped = 2
	capsPaused  = 3
	capsPlaying = 4
)

// Status is what the player says it is doing.
type Status struct {
	// Revision is handed back on the next request, which then waits until
	// the player has something newer to say.
	Revision uint32

	Playing bool
	Paused  bool // neither Playing nor Paused is stopped

	Title  string
	Artist string
	Album  string
	Genre  string

	// AlbumID identifies the album in the player's library. It is what says
	// the cover has changed.
	AlbumID uint64
	// NowPlaying is the player's 16-byte id for what is playing: database,
	// playlist, container item and track.
	NowPlaying []byte

	DurationMS  uint32
	RemainingMS uint32
}

// PositionMS is how far into the track playback is.
func (s Status) PositionMS() uint32 {
	if s.RemainingMS > s.DurationMS {
		return 0
	}
	return s.DurationMS - s.RemainingMS
}

// Client talks to one player.
type Client struct {
	Host string
	Port int
	HTTP *http.Client

	session uint64
}

// NewClient builds a client. Requests carry no overall timeout, because the
// one that matters most is meant to hang: see Wait. Dialling and the response
// headers of an ordinary request are bounded instead.
func NewClient(host string, port int) *Client {
	if port == 0 {
		port = DefaultPort
	}
	return &Client{
		Host: host,
		Port: port,
		HTTP: &http.Client{Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
				// A player that sleeps or loses power says nothing. Keepalive
				// is what eventually notices, on a request built to wait.
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:    2,
			IdleConnTimeout: 90 * time.Second,
		}},
	}
}

func (c *Client) url(path string) string {
	return fmt.Sprintf("http://%s/%s", net.JoinHostPort(c.Host, fmt.Sprint(c.Port)), path)
}

// get performs one request and decodes the DMAP answer.
func (c *Client) get(ctx context.Context, path string) ([]Item, error) {
	body, err := c.raw(ctx, path, 1<<20)
	if err != nil {
		return nil, err
	}
	return Decode(body)
}

func (c *Client) raw(ctx context.Context, path string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	// What Apple's Remote sends. The player answers a client that looks like
	// a remote; Viewer-Only-Client is the one known to be required.
	req.Header.Set("Viewer-Only-Client", "1")
	req.Header.Set("Client-DAAP-Version", "3.13")
	req.Header.Set("Client-ATV-Sharing-Version", "1.2")
	req.Header.Set("Client-iTunes-Sharing-Version", "3.15")
	req.Header.Set("User-Agent", "Remote/1021")
	req.Header.Set("Accept", "*/*")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, &StatusError{Code: resp.StatusCode, Status: resp.Status}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("dacp: answer larger than %d bytes", limit)
	}
	return body, nil
}

// StatusError is an HTTP answer other than success. A 403 on login means the
// player does not know this remote: it was never paired, or was removed.
type StatusError struct {
	Code   int
	Status string
}

func (e *StatusError) Error() string { return "dacp: the player answered " + e.Status }

// Login opens a session with a pairing GUID. It is the only credential: no
// Apple ID, no Home Sharing.
func (c *Client) Login(ctx context.Context, guid string) error {
	items, err := c.get(ctx, "login?pairing-guid=0x"+guid+"&hasFP=1")
	if err != nil {
		return err
	}
	id, ok := Find(items, "mlid")
	if !ok {
		return fmt.Errorf("dacp: login answered without a session id")
	}
	session, ok := id.Uint()
	if !ok {
		return fmt.Errorf("dacp: unreadable session id")
	}
	c.session = session
	return nil
}

// Now asks what is playing and gets an answer at once.
func (c *Client) Now(ctx context.Context) (Status, error) {
	return c.status(ctx, 1)
}

// Wait asks what is playing and does not get an answer until it has changed
// since revision. This is the push notification: the request simply hangs
// until the track changes, playback pauses, or the connection dies.
//
// The caller's context is the only deadline. There is no telling a quiet
// evening from a player that has gone away except by how long it has been.
func (c *Client) Wait(ctx context.Context, revision uint32) (Status, error) {
	return c.status(ctx, revision)
}

func (c *Client) status(ctx context.Context, revision uint32) (Status, error) {
	items, err := c.get(ctx, fmt.Sprintf("ctrl-int/1/playstatusupdate?revision-number=%d&session-id=%d", revision, c.session))
	if err != nil {
		return Status{}, err
	}
	root, ok := Find(items, "cmst")
	if !ok {
		return Status{}, fmt.Errorf("dacp: no play status in the answer")
	}
	return parseStatus(root.Children), nil
}

func parseStatus(items []Item) Status {
	var s Status
	for _, item := range items {
		n, _ := item.Uint()
		switch item.Tag {
		case "cmsr":
			s.Revision = uint32(n)
		case "caps":
			s.Playing, s.Paused = n == capsPlaying, n == capsPaused
		case "cann":
			s.Title = item.String()
		case "cana":
			s.Artist = item.String()
		case "canl":
			s.Album = item.String()
		case "cang":
			s.Genre = item.String()
		case "asai":
			s.AlbumID = n
		case "canp":
			s.NowPlaying = append([]byte(nil), item.Data...)
		case "cast":
			s.DurationMS = uint32(n)
		case "cant":
			s.RemainingMS = uint32(n)
		}
	}
	return s
}

// Artwork fetches the cover of what is playing, at up to size pixels square.
// It returns nil, not an error, when the player has none to give.
func (c *Client) Artwork(ctx context.Context, size int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	body, err := c.raw(ctx, fmt.Sprintf("ctrl-int/1/nowplayingartwork?mw=%d&mh=%d&session-id=%d", size, size, c.session), 8<<20)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	return body, nil
}
