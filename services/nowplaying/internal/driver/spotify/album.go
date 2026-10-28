package spotify

// play_album on Spotify: a demo, not yet a decision. ADR-0040 sends the
// command to iTunes; with NP_ALBUM_SOURCE=spotify it comes here instead, and
// the album is found in Spotify's catalog and played on the device that is
// playing here. As with iTunes, a miss is a failed command, never a nearer
// guess: the album must be the artist's and carry the name asked for, give
// or take what Spotify adds after it ("Van Halen (Remastered)").

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

// ErrNoAlbum means Spotify has none of the albums asked for.
var ErrNoAlbum = errors.New("spotify: has none of the albums asked for")

// playAlbum plays the first of the command's album and its alternatives
// (ADR-0045) that Spotify has. With a track here it plays where that is;
// with none, on a device that counts and is open, since asking for an album
// is how music is started and not only changed. A device elsewhere is still
// never chosen.
func (d *Driver) playAlbum(ctx context.Context, c contract.Command, here bool) error {
	play := "/v1/me/player/play"
	if !here {
		id, err := d.openDevice(ctx)
		if err != nil {
			return err
		}
		play += "?" + url.Values{"device_id": {id}}.Encode()
	}
	for _, album := range append([]string{c.Album}, c.Instead...) {
		uri, err := d.findAlbum(ctx, c.Artist, album)
		if err != nil {
			return err
		}
		if uri == "" {
			continue
		}
		body, _ := json.Marshal(map[string]string{"context_uri": uri})
		if _, err := d.send(ctx, http.MethodPut, play, body); err != nil {
			return err
		}
		d.logger().Info("playing an album on Spotify", "artist", c.Artist, "album", album, "uri", uri)
		select {
		case d.pokes() <- struct{}{}:
		default:
		}
		return nil
	}
	return ErrNoAlbum
}

// openDevice is the id of a device that counts and has Spotify open, the
// active one first.
func (d *Driver) openDevice(ctx context.Context) (string, error) {
	answer, err := d.send(ctx, http.MethodGet, "/v1/me/player/devices", nil)
	if err != nil {
		return "", err
	}
	var open struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(answer, &open); err != nil {
		return "", fmt.Errorf("spotify: a devices answer that does not read: %w", err)
	}
	id := ""
	for _, dev := range open.Devices {
		if dev.ID == "" || !d.counts(dev.Name) {
			continue
		}
		if dev.Active {
			return dev.ID, nil
		}
		if id == "" {
			id = dev.ID
		}
	}
	if id == "" {
		return "", ErrNotHere
	}
	return id, nil
}

// findAlbum is the album's URI in Spotify's catalog, or "" when it has none
// by that artist and name.
func (d *Driver) findAlbum(ctx context.Context, artist, album string) (string, error) {
	q := url.Values{
		"q":     {fmt.Sprintf("album:%q artist:%q", album, artist)},
		"type":  {"album"},
		"limit": {"10"},
	}
	answer, err := d.send(ctx, http.MethodGet, "/v1/search?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	var found struct {
		Albums struct {
			Items []struct {
				Name    string `json:"name"`
				URI     string `json:"uri"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(answer, &found); err != nil {
		return "", fmt.Errorf("spotify: a search answer that does not read: %w", err)
	}

	// An exact name first, then one with only Spotify's addition after it.
	var near string
	for _, a := range found.Albums.Items {
		if len(a.Artists) == 0 || !strings.EqualFold(a.Artists[0].Name, artist) {
			continue
		}
		switch name := strings.ToLower(a.Name); {
		case name == strings.ToLower(album):
			return a.URI, nil
		case near == "" && suffixed(name, strings.ToLower(album)):
			near = a.URI
		}
	}
	if near == "" {
		var seen []string
		for _, a := range found.Albums.Items {
			by := ""
			if len(a.Artists) > 0 {
				by = a.Artists[0].Name
			}
			seen = append(seen, a.Name+" / "+by)
		}
		d.logger().Info("not on Spotify by that name", "artist", artist, "album", album, "seen", seen)
	}
	return near, nil
}

// suffixed is whether name is want followed by an addition in parentheses,
// brackets or after a dash.
func suffixed(name, want string) bool {
	rest, ok := strings.CutPrefix(name, want)
	if !ok {
		return false
	}
	rest = strings.TrimSpace(rest)
	return strings.HasPrefix(rest, "(") || strings.HasPrefix(rest, "[") || strings.HasPrefix(rest, "- ")
}

// send asks Spotify one thing with the account's token, once more with a
// fresh one if the first is refused, and returns the answer's body.
func (d *Driver) send(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	for try := 0; ; try++ {
		token, err := d.Tokens.Access(ctx)
		if err != nil {
			return nil, err
		}
		var r io.Reader = http.NoBody
		if body != nil {
			r = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, d.api()+path, r)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := d.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		answer, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized && try == 0:
			d.Tokens.Invalidate()
			continue
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, errors.New("spotify: refused a freshly issued access token")
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return answer, nil
		}
		resp.Body = io.NopCloser(bytes.NewReader(answer))
		return nil, refusal(resp)
	}
}
