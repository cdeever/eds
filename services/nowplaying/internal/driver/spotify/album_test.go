package spotify

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

const searched = `{"albums":{"items":[
 {"name":"Van Halen II","uri":"spotify:album:two","artists":[{"name":"Van Halen"}]},
 {"name":"Van Halen (Remastered)","uri":"spotify:album:one","artists":[{"name":"Van Halen"}]},
 {"name":"Van Halen","uri":"spotify:album:tribute","artists":[{"name":"A Tribute Band"}]}]}}`

func albumCommand(album string, instead ...string) contract.Command {
	return contract.Command{V: contract.Version, Command: contract.PlayAlbum, Artist: "Van Halen", Album: album, Instead: instead}
}

func TestAnAlbumIsFoundAndPlayed(t *testing.T) {
	d, s, _ := commanded(t, here{track: true}, http.StatusOK, http.StatusNoContent)
	s.body = searched
	if err := d.Command(context.Background(), albumCommand("Van Halen")); err != nil {
		t.Fatal(err)
	}
	if len(s.asked) != 2 || !strings.HasPrefix(s.asked[0], "GET /v1/search?") || s.asked[1] != "PUT /v1/me/player/play" {
		t.Fatalf("asked %v", s.asked)
	}
	if s.lengths[1] == 0 {
		t.Error("the play request named no album")
	}
}

func TestAnAlbumSpotifyHasNotIsAMiss(t *testing.T) {
	d, s, _ := commanded(t, here{track: true}, http.StatusOK)
	s.body = searched
	err := d.Command(context.Background(), albumCommand("1984", "Women and Children First"))
	if !errors.Is(err, ErrNoAlbum) {
		t.Fatalf("err %v, want ErrNoAlbum", err)
	}
	for _, a := range s.asked {
		if strings.HasPrefix(a, "PUT") {
			t.Errorf("played something: %v", s.asked)
		}
	}
}

func TestWithNothingHereAnAlbumIsPlayedOnAnOpenDeviceThatCounts(t *testing.T) {
	d, s, _ := commanded(t, here{}, http.StatusOK, http.StatusOK, http.StatusNoContent)
	s.body = `{"devices":[{"id":"car","name":"Car","is_active":true},{"id":"pc","name":"study pc"}],` + searched[1:]
	if err := d.Command(context.Background(), albumCommand("Van Halen")); err != nil {
		t.Fatal(err)
	}
	if len(s.asked) != 3 || s.asked[0] != "GET /v1/me/player/devices" || s.asked[2] != "PUT /v1/me/player/play?device_id=pc" {
		t.Fatalf("asked %v", s.asked)
	}
}

func TestWithNothingHereAndNoDeviceThatCountsNoAlbumIsPlayed(t *testing.T) {
	d, s, _ := commanded(t, here{}, http.StatusOK)
	s.body = `{"devices":[{"id":"car","name":"Car","is_active":true}]}`
	if err := d.Command(context.Background(), albumCommand("Van Halen")); !errors.Is(err, ErrNotHere) {
		t.Fatalf("err %v", err)
	}
	if len(s.asked) != 1 {
		t.Errorf("asked %v", s.asked)
	}
}

func TestSuffixed(t *testing.T) {
	for name, want := range map[string]bool{
		"van halen (remastered)": true, "van halen [deluxe]": true, "van halen - 2015 remaster": true,
		"van halen ii": false, "van halen": false,
	} {
		if got := suffixed(name, "van halen"); got != want {
			t.Errorf("suffixed(%q) = %v", name, got)
		}
	}
}
