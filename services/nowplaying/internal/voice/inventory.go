package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Inventory is eds-inventory, the owner's music in every format, asked over
// HTTP for what a request needs (ADR-0047). It does the matching that the
// library index did in voiced (ADR-0046), with the same thresholds, and
// answers null for what is not near enough.
//
// It runs beside voiced on the services workload, so it is not an outside
// service and is not counted.
type Inventory struct {
	// URL is where it answers, such as http://127.0.0.1:8740.
	URL  string
	HTTP *http.Client
}

// Owner is the inventory itself: whether it can be reached is learned by
// asking it.
func (i *Inventory) Owner() (Owner, error) { return i, nil }

// The contract's answers, as much of them as voiced reads.
type invArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type invAlbum struct {
	Title string `json:"title"`
}

func (i *Inventory) FindArtist(ctx context.Context, heard string) (*OwnedArtist, float64, error) {
	var ans struct {
		Score  float64    `json:"score"`
		Artist *invArtist `json:"artist"`
	}
	if err := i.get(ctx, "/v1/artists/find?"+url.Values{"name": {heard}}.Encode(), &ans); err != nil {
		return nil, 0, err
	}
	if ans.Artist == nil {
		return nil, ans.Score, nil
	}
	return &OwnedArtist{ID: ans.Artist.ID, Name: ans.Artist.Name}, ans.Score, nil
}

func (i *Inventory) FindAlbum(ctx context.Context, a *OwnedArtist, title string) (string, error) {
	return i.album(ctx, a, url.Values{"title": {title}})
}

func (i *Inventory) Owned(ctx context.Context, a *OwnedArtist, titles ...string) (string, error) {
	if len(titles) == 0 {
		return "", nil
	}
	return i.album(ctx, a, url.Values{"title": titles, "studio": {"true"}})
}

func (i *Inventory) album(ctx context.Context, a *OwnedArtist, q url.Values) (string, error) {
	var ans struct {
		Album *invAlbum `json:"album"`
	}
	if err := i.get(ctx, "/v1/artists/"+url.PathEscape(a.ID)+"/albums/find?"+q.Encode(), &ans); err != nil {
		return "", err
	}
	if ans.Album == nil {
		return "", nil
	}
	return ans.Album.Title, nil
}

func (i *Inventory) ByYear(ctx context.Context, a *OwnedArtist) ([]string, error) {
	var ans struct {
		Albums []invAlbum `json:"albums"`
	}
	if err := i.get(ctx, "/v1/artists/"+url.PathEscape(a.ID)+"/albums/by-year", &ans); err != nil {
		return nil, err
	}
	var out []string
	for _, al := range ans.Albums {
		out = append(out, al.Title)
	}
	return out, nil
}

// get asks the inventory, and refuses an answer of a version this program
// does not speak.
func (i *Inventory) get(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(i.URL, "/")+path, nil)
	if err != nil {
		return err
	}
	client := i.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		V     int    `json:"v"`
		Error string `json:"error"`
	}
	raw := json.NewDecoder(resp.Body)
	var msg json.RawMessage
	if err := raw.Decode(&msg); err != nil {
		return fmt.Errorf("inventory: %s: %w", resp.Status, err)
	}
	if err := json.Unmarshal(msg, &body); err != nil {
		return fmt.Errorf("inventory: %w", err)
	}
	if body.V != 1 {
		return fmt.Errorf("inventory speaks version %d, this program 1", body.V)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inventory: %s: %s", resp.Status, body.Error)
	}
	return json.Unmarshal(msg, into)
}
