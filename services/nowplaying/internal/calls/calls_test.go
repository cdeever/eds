package calls

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestASummaryAddsUpAMinute(t *testing.T) {
	m := New()
	m.Record("discogs", OK, 100*time.Millisecond)
	m.Record("discogs", Limited, 300*time.Millisecond)
	m.Record("discogs", Failed, 200*time.Millisecond)
	m.Remaining("discogs", 40)
	m.Remaining("discogs", 12)
	m.Remaining("discogs", 30)
	m.Record("deezer", OK, time.Millisecond)

	got := m.Take()
	if len(got) != 2 || got[0].API != "deezer" || got[1].API != "discogs" {
		t.Fatalf("summaries = %+v, want deezer then discogs", got)
	}
	d := got[1]
	if d.Calls != 3 || d.Limited != 1 || d.Failed != 1 || d.SlowestMS != 300 || d.MeanMS != 200 {
		t.Fatalf("discogs = %+v", d)
	}
	// The least left is how close the minute came.
	if d.Remaining == nil || *d.Remaining != 12 {
		t.Fatalf("remaining = %v, want 12", d.Remaining)
	}
	if got[0].Remaining != nil {
		t.Fatal("a service that says nothing of an allowance was given one")
	}
}

func TestAMinuteWithNoCallsSaysNothing(t *testing.T) {
	m := New()
	m.Record("discogs", OK, time.Millisecond)
	m.Take()
	if got := m.Take(); len(got) != 0 {
		t.Fatalf("summaries = %+v, want none", got)
	}
	// What is left is not a call.
	m.Remaining("discogs", 5)
	if got := m.Take(); len(got) != 0 {
		t.Fatalf("summaries = %+v, want none", got)
	}
}

func TestTheLastMinuteIsHandedOverOnTheWayOut(t *testing.T) {
	m := New()
	m.Record("spotify", OK, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var got []Summary
	m.Run(ctx, time.Hour, func(s Summary) { got = append(got, s) })
	if len(got) != 1 || got[0].API != "spotify" {
		t.Fatalf("summaries = %+v", got)
	}
}

func TestTheClientCountsWhatItAsks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/limited":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/broken":
			w.WriteHeader(http.StatusBadGateway)
		case "/moved":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			w.Header().Set("X-Discogs-Ratelimit-Remaining", "57")
		}
	}))
	defer server.Close()

	m := New()
	client := m.Client(5 * time.Second)
	for _, path := range []string{"/ok", "/limited", "/missing", "/broken", "/moved"} {
		resp, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// A server that cannot be reached is a call all the same.
	server.Close()
	if _, err := client.Get(server.URL + "/ok"); err == nil {
		t.Fatal("a closed server answered")
	}

	got := m.Take()
	if len(got) != 1 {
		t.Fatalf("summaries = %+v, want one", got)
	}
	s := got[0]
	// Five asked for, one of them answered with a redirect that was
	// followed, and one that reached nobody.
	if s.API != "127.0.0.1" || s.Calls != 7 || s.Limited != 1 || s.Failed != 2 {
		t.Fatalf("summary = %+v", s)
	}
	if s.Remaining == nil || *s.Remaining != 57 {
		t.Fatalf("remaining = %v, want 57", s.Remaining)
	}
}

func TestWhoseAHostIs(t *testing.T) {
	for host, want := range map[string]string{
		"api.discogs.com":         "discogs",
		"i.discogs.com":           "discogs-images",
		"api.deezer.com":          "deezer",
		"e-cdns-images.dzcdn.net": "deezer-images",
		"itunes.apple.com":        "itunes",
		"is1-ssl.mzstatic.com":    "itunes-images",
		"musicbrainz.org":         "musicbrainz",
		"coverartarchive.org":     "coverartarchive",
		"ia800.us.archive.org":    "coverartarchive",
		"api.spotify.com":         "spotify",
		"accounts.spotify.com":    "spotify",
		"API.Spotify.com":         "spotify",
		"notdiscogs.com":          "notdiscogs.com",
		"somebody.example":        "somebody.example",
	} {
		if got := Name(host); got != want {
			t.Errorf("Name(%q) = %q, want %q", host, got, want)
		}
	}
}
