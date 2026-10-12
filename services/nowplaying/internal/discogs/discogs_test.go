package discogs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// A PNG's first bytes: enough to be sniffed as a picture.
var png = []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32))

// shelf stands in for Discogs: a collection of records, each with a
// tracklist, and a count of what was asked for.
type shelf struct {
	t      *testing.T
	server *httptest.Server
	// images is another host, as Discogs' pictures are.
	images *httptest.Server
	token  string // what the API insists on, if anything
	medium string // what the next record added is on

	mu       sync.Mutex
	records  []string            // one basic_information each, as JSON, in order
	tracks   map[int]string      // release id -> tracklist JSON
	asked    map[string]int      // path -> times asked
	auth     map[string][]string // path -> Authorization headers seen
	perPage  int
	down     bool
	limit    int           // answer 429 this many times
	slow     chan struct{} // releases wait on this, if set
	pictures int
}

func newShelf(t *testing.T) *shelf {
	s := &shelf{t: t, medium: "Vinyl", tracks: map[int]string{}, asked: map[string]int{}, auth: map[string][]string{}, perPage: 100}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	s.images = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	t.Cleanup(s.images.Close)
	return s
}

// add puts a record on the shelf. Tracks are titles; extra is spliced into
// its basic_information.
func (s *shelf) add(id int, artist, title string, year int, formats string, tracks ...string) {
	if formats == "" {
		formats = `"LP","Album"`
	}
	s.records = append(s.records, fmt.Sprintf(
		`{"id":%d,"basic_information":{"id":%d,"title":%q,"year":%d,"cover_image":"%s/img/%d.png","artists":[{"name":%q}],"formats":[{"name":%q,"descriptions":[%s]}]}}`,
		id, id, title, year, s.images.URL, id, artist, s.medium, formats))
	var list []string
	for _, title := range tracks {
		list = append(list, fmt.Sprintf(`{"position":"","type_":"track","title":%q}`, title))
	}
	s.tracks[id] = `{"tracklist":[` + strings.Join(list, ",") + `]}`
}

func (s *shelf) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.asked[r.URL.Path]++
	s.auth[r.URL.Path] = append(s.auth[r.URL.Path], r.Header.Get("Authorization"))
	down, slow := s.down, s.slow
	limited := s.limit > 0 && strings.HasPrefix(r.URL.Path, "/releases/")
	if limited {
		s.limit--
	}
	s.mu.Unlock()

	if r.Header.Get("User-Agent") == "" {
		http.Error(w, "say who you are", http.StatusBadRequest)
		return
	}
	switch {
	case down:
		http.Error(w, "down", http.StatusServiceUnavailable)
	case strings.HasPrefix(r.URL.Path, "/img/"):
		s.mu.Lock()
		s.pictures++
		s.mu.Unlock()
		w.Write(png)
	case s.token != "" && r.Header.Get("Authorization") != "Discogs token="+s.token:
		http.Error(w, `{"message":"You must authenticate to access this resource."}`, http.StatusForbidden)
	case limited:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	case r.URL.Path == "/users/someone/collection/folders/0/releases":
		page := 1
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		pages := (len(s.records) + s.perPage - 1) / s.perPage
		from, to := (page-1)*s.perPage, page*s.perPage
		if to > len(s.records) {
			to = len(s.records)
		}
		fmt.Fprintf(w, `{"pagination":{"page":%d,"pages":%d},"releases":[%s]}`, page, pages, strings.Join(s.records[from:to], ","))
	case strings.HasPrefix(r.URL.Path, "/releases/"):
		if slow != nil {
			<-slow
		}
		var id int
		fmt.Sscanf(r.URL.Path, "/releases/%d", &id)
		if list, ok := s.tracks[id]; ok {
			io.WriteString(w, list)
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *shelf) times(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asked[path]
}

func (s *shelf) collection() *Collection {
	return &Collection{Base: s.server.URL, User: "someone", Token: s.token, HTTP: s.server.Client(), UserAgent: "test", Logger: quiet}
}

const listing = "/users/someone/collection/folders/0/releases"

func find(t *testing.T, c *Collection, artist, title string) resolve.Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := c.Find(ctx, artist, title, "")
	if err != nil {
		t.Fatalf("%s - %s: %v", artist, title, err)
	}
	return got
}

func TestTheRecordATrackIsOn(t *testing.T) {
	s := newShelf(t)
	s.add(10, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio", "Take Me Home")
	s.add(11, "Chicago (2)", "Chicago Transit Authority", 1969, "", "Introduction", "Does Anybody Really Know What Time It Is?")
	s.add(12, "Beatles, The", "Abbey Road", 1969, "", "Come Together")
	c := s.collection()

	got := find(t, c, "Phil Collins", "Take Me Home")
	if got.Album != "No Jacket Required" || got.Cover == nil || got.Cover.From != "discogs" || got.Cover.MIME != "image/png" {
		t.Fatalf("got %q, cover %+v", got.Album, got.Cover)
	}

	// Discogs' ways of writing an artist, and a recogniser's of a title.
	if got := find(t, c, "Chicago", "Does Anybody Really Know What Time It Is? (2002 Remaster)"); got.Album != "Chicago Transit Authority" {
		t.Fatalf("got %q", got.Album)
	}
	if got := find(t, c, "The Beatles", "Come Together"); got.Album != "Abbey Road" {
		t.Fatalf("got %q", got.Album)
	}

	// Only the records of the artist asked about were read.
	if s.times("/releases/10") != 1 || s.times("/releases/11") != 1 || s.times("/releases/12") != 1 {
		t.Fatalf("asked %v", s.asked)
	}
}

func TestWhatTheCollectionDoesNotHave(t *testing.T) {
	s := newShelf(t)
	s.add(10, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio", "Take Me Home")
	c := s.collection()

	for _, q := range [][2]string{
		{"Genesis", "Mama"},                    // an artist with no record here
		{"Phil Collins", "In the Air Tonight"}, // a record the owner does not have
		{"Phil Collins", ""},
		{"", "Sussudio"},
	} {
		if _, err := c.Find(context.Background(), q[0], q[1], ""); !errors.Is(err, resolve.ErrNotFound) {
			t.Errorf("%q - %q: %v, want ErrNotFound", q[0], q[1], err)
		}
	}
	// Nothing was read for an artist with no record.
	if s.times("/releases/10") != 1 {
		t.Fatalf("asked %v", s.asked)
	}
}

func TestATitleIsNotMatchedByItsFirstLetters(t *testing.T) {
	s := newShelf(t)
	s.add(20, "Chicago", "Chicago Transit Authority", 1969, "", "Introduction")
	s.add(21, "Chicago", "Hot Streets", 1978, "", "Alive Again", "Take a Chance")
	c := s.collection()

	if _, err := c.Find(context.Background(), "Chicago", "Intro", ""); !errors.Is(err, resolve.ErrNotFound) {
		t.Fatalf("\"Intro\" was taken for \"Introduction\": %v", err)
	}
	// A longer title that holds the one asked for is the same track.
	s.add(22, "Chicago", "Chicago VII", 1974, "", "Wishing You Were Here (With The Beach Boys)", "Call on Me - Single Version")
	c = s.collection()
	if got := find(t, c, "Chicago", "Call on Me"); got.Album != "Chicago VII" {
		t.Fatalf("got %q", got.Album)
	}
}

func TestATrackOnTwoRecords(t *testing.T) {
	s := newShelf(t)
	s.add(30, "Phil Collins", "...Hits", 1998, `"LP","Compilation"`, "Sussudio", "In the Air Tonight", "Another Day in Paradise")
	s.add(31, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio", "Take Me Home")
	s.add(32, "Phil Collins", "No Jacket Required", 2016, `"LP","Album","Reissue"`, "Sussudio", "Take Me Home")
	c := s.collection()

	// The album before the compilation, and the first pressing before the
	// reissue.
	if got := find(t, c, "Phil Collins", "Sussudio"); got.Album != "No Jacket Required" || c.last != 31 {
		t.Fatalf("got %q, record %d", got.Album, c.last)
	}

	// Always: with the compilation playing, a song that is also on its own
	// album is shown as that album. Only a song with no album here is shown
	// as the compilation.
	if got := find(t, c, "Phil Collins", "Another Day in Paradise"); got.Album != "...Hits" {
		t.Fatalf("got %q", got.Album)
	}
	if got := find(t, c, "Phil Collins", "Sussudio"); got.Album != "No Jacket Required" {
		t.Fatalf("got %q; its own album is here", got.Album)
	}
	// Even when the source names the compilation.
	ctx := context.Background()
	if got, err := c.Find(ctx, "Phil Collins", "Sussudio", "...Hits"); err != nil || got.Album != "No Jacket Required" {
		t.Fatalf("got %q, %v", got.Album, err)
	}

	// Between two albums, what the source called it decides, edition notes
	// and all; and once a record is playing, a track on both stays on it.
	s.add(33, "Phil Collins", "Serious Hits... Live!", 1990, "", "Sussudio", "Take Me Home")
	c = s.collection()
	if got, err := c.Find(ctx, "Phil Collins", "Sussudio", "Serious Hits... Live! (Remastered)"); err != nil || got.Album != "Serious Hits... Live!" {
		t.Fatalf("got %q, %v", got.Album, err)
	}
	if got := find(t, c, "Phil Collins", "Take Me Home"); got.Album != "Serious Hits... Live!" {
		t.Fatalf("got %q; the record already playing has this track", got.Album)
	}
	// And the name is only a hint: a track is found without a record of it.
	if got, err := c.Find(ctx, "Phil Collins", "In the Air Tonight", "Face Value"); err != nil || got.Album != "...Hits" {
		t.Fatalf("got %q, %v", got.Album, err)
	}
}

func TestARecordBeforeTheSameAlbumOnADisc(t *testing.T) {
	s := newShelf(t)
	s.medium = "CD"
	s.add(35, "Phil Collins", "Hello, I Must Be Going!", 1982, "", "I Don't Care Anymore")
	s.medium = "Vinyl"
	s.add(36, "Phil Collins", "Hello, I Must Be Going!", 2016, `"LP","Album","Reissue"`, "I Don't Care Anymore")
	c := s.collection()

	// The disc is the earlier, and it is not what is on a turntable.
	if find(t, c, "Phil Collins", "I Don't Care Anymore"); c.last != 36 {
		t.Fatalf("record %d", c.last)
	}
}

func TestTheListIsReadOnceAndReadAgainWhenItIsOld(t *testing.T) {
	s := newShelf(t)
	s.perPage = 2
	for i := 1; i <= 5; i++ {
		s.add(40+i, fmt.Sprintf("Artist %d", i), fmt.Sprintf("Record %d", i), 1970+i, "", fmt.Sprintf("Track of %d", i))
	}
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	c := s.collection()
	c.Now = func() time.Time { return now }

	// Every page, and the record on the last of them.
	if got := find(t, c, "Artist 5", "Track of 5"); got.Album != "Record 5" {
		t.Fatalf("got %q", got.Album)
	}
	if s.times(listing) != 3 {
		t.Fatalf("the listing was asked for %d times, want 3 pages", s.times(listing))
	}
	find(t, c, "Artist 1", "Track of 1")
	if s.times(listing) != 3 {
		t.Fatal("the listing was read again within its six hours")
	}

	// A record bought since is found once the list is old.
	s.mu.Lock()
	s.perPage = 100
	s.mu.Unlock()
	s.add(50, "Newcomer", "First", 2026, "", "Opener")
	now = now.Add(7 * time.Hour)
	if got := find(t, c, "Newcomer", "Opener"); got.Album != "First" {
		t.Fatalf("got %q", got.Album)
	}

	// And a list that cannot be read again is kept. Tracklists already read
	// are not asked for twice.
	now = now.Add(7 * time.Hour)
	s.mu.Lock()
	s.down = true
	s.mu.Unlock()
	if got := find(t, c, "Artist 1", "Track of 1"); got.Album != "Record 1" {
		t.Fatalf("got %q", got.Album)
	}
	if s.times("/releases/41") != 1 {
		t.Fatalf("a tracklist was read %d times", s.times("/releases/41"))
	}
}

func TestACollectionThatCannotBeRead(t *testing.T) {
	s := newShelf(t)
	s.token = "secret"
	s.add(60, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio")

	// Private, and no token: said in so many words, and not "not found".
	c := s.collection()
	c.Token = ""
	_, err := c.Find(context.Background(), "Phil Collins", "Sussudio", "")
	if err == nil || errors.Is(err, resolve.ErrNotFound) || !strings.Contains(err.Error(), "token") {
		t.Fatalf("got %v", err)
	}

	// With it, read. The token goes to the API and not with the picture.
	c = s.collection()
	if got := find(t, c, "Phil Collins", "Sussudio"); got.Cover == nil {
		t.Fatal("no cover")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if got := s.auth["/img/60.png"]; len(got) != 1 || got[0] != "" {
		t.Fatalf("the picture was asked for with %q", got)
	}
	if got := s.auth["/releases/60"]; len(got) != 1 || got[0] != "Discogs token=secret" {
		t.Fatalf("the release was asked for with %q", got)
	}
}

func TestAnArtistWithManyRecordsIsNotWaitedFor(t *testing.T) {
	s := newShelf(t)
	s.slow = make(chan struct{})
	s.add(70, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio")
	c := s.collection()

	// Whoever asks has only so long. They are told it is not ready - which
	// is not "the owner does not have it".
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, err := c.Find(ctx, "Phil Collins", "Sussudio", "")
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, resolve.ErrNotFound) {
		t.Fatalf("got %v", err)
	}

	// The reading went on without them, and the next track finds it done.
	close(s.slow)
	if got := find(t, c, "Phil Collins", "Sussudio"); got.Album != "No Jacket Required" {
		t.Fatalf("got %q", got.Album)
	}
	if s.times("/releases/70") != 1 {
		t.Fatalf("the tracklist was asked for %d times", s.times("/releases/70"))
	}
}

func TestBeingToldToWait(t *testing.T) {
	s := newShelf(t)
	s.limit = 1
	s.add(80, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio")
	c := s.collection()

	if got := find(t, c, "Phil Collins", "Sussudio"); got.Album != "No Jacket Required" {
		t.Fatalf("got %q", got.Album)
	}
	if s.times("/releases/80") != 2 {
		t.Fatalf("asked %d times, want once refused and once answered", s.times("/releases/80"))
	}
}

func TestOneCoverIsKeptAndNoMore(t *testing.T) {
	s := newShelf(t)
	s.add(90, "Phil Collins", "No Jacket Required", 1985, "", "Sussudio", "Take Me Home")
	s.add(91, "Chicago", "Chicago Transit Authority", 1969, "", "Introduction")
	c := s.collection()

	find(t, c, "Phil Collins", "Sussudio")
	find(t, c, "Phil Collins", "Take Me Home")
	if s.pictures != 1 {
		t.Fatalf("one record's cover was fetched %d times", s.pictures)
	}
	find(t, c, "Chicago", "Introduction")
	find(t, c, "Phil Collins", "Sussudio")
	if s.pictures != 3 {
		t.Fatalf("%d pictures fetched, want 3: only the last is kept", s.pictures)
	}
}

func TestArtistName(t *testing.T) {
	for in, want := range map[string]string{
		"Chicago (2)":        "Chicago",
		"Beatles, The":       "The Beatles",
		"Phil Collins*":      "Phil Collins",
		"Earth, Wind & Fire": "Earth, Wind & Fire",
		"Sunn O)))":          "Sunn O)))",
	} {
		if got := artistName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
