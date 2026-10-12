package spotifyauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The pair in RFC 7636, appendix B.
func TestChallengeMatchesTheRFC(t *testing.T) {
	got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("challenge = %q", got)
	}
}

func TestVerifiersAndStatesAreUnguessableAndURLSafe(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		v, err := NewVerifier()
		if err != nil || len(v) < 43 || len(v) > 128 || seen[v] {
			t.Fatalf("verifier %q (%d chars), %v", v, len(v), err)
		}
		if strings.ContainsAny(v, "+/=") {
			t.Fatalf("verifier %q is not base64url", v)
		}
		seen[v] = true
	}
	a, _ := NewState()
	b, _ := NewState()
	if a == b || len(a) < 20 {
		t.Errorf("states %q and %q", a, b)
	}
}

func TestTheAuthorizeURLAsksOnlyToRead(t *testing.T) {
	raw := Spotify.AuthorizeURL("client123", "state456", "challenge789")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()

	if u.Host != "accounts.spotify.com" || q.Get("response_type") != "code" {
		t.Errorf("url = %s", raw)
	}
	if q.Get("client_id") != "client123" || q.Get("state") != "state456" {
		t.Errorf("client or state lost: %v", q)
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != "challenge789" {
		t.Errorf("pkce lost: %v", q)
	}
	// The address by number: Spotify refuses "localhost".
	if q.Get("redirect_uri") != "http://127.0.0.1:8898/callback" {
		t.Errorf("redirect = %q", q.Get("redirect_uri"))
	}
	// Reading what is playing, and nothing that changes it.
	scope := q.Get("scope")
	if scope != "user-read-playback-state user-read-currently-playing" {
		t.Errorf("scope = %q", scope)
	}
	if strings.Contains(scope, "modify") || strings.Contains(scope, "streaming") {
		t.Errorf("asked for more than reading: %q", scope)
	}
}

// tokens is a stand-in for Spotify's token endpoint.
type tokens struct {
	t      *testing.T
	status int
	body   string
	forms  []url.Values
}

func (s *tokens) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		s.t.Errorf("%s with %q", r.Method, r.Header.Get("Content-Type"))
	}
	if r.Header.Get("Authorization") != "" {
		s.t.Error("a client secret was sent; PKCE has none")
	}
	_ = r.ParseForm()
	s.forms = append(s.forms, r.PostForm)
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(s.body))
}

func endpoint(t *testing.T, s *tokens) (Endpoints, *http.Client) {
	t.Helper()
	s.t = t
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return Endpoints{Token: srv.URL}, srv.Client()
}

func TestExchangeSendsTheVerifierAndNoSecret(t *testing.T) {
	s := &tokens{body: `{"access_token":"AT","refresh_token":"RT","expires_in":3600}`}
	e, client := endpoint(t, s)

	grant, err := e.Exchange(context.Background(), client, "client123", "the-code", "the-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if grant.AccessToken != "AT" || grant.RefreshToken != "RT" || grant.ExpiresIn != time.Hour {
		t.Errorf("grant = %+v", grant)
	}
	form := s.forms[0]
	if form.Get("grant_type") != "authorization_code" || form.Get("code") != "the-code" ||
		form.Get("code_verifier") != "the-verifier" || form.Get("client_id") != "client123" ||
		form.Get("redirect_uri") != RedirectURI || form.Get("client_secret") != "" {
		t.Errorf("form = %v", form)
	}
}

func TestAnEndedAuthorizationIsDistinct(t *testing.T) {
	s := &tokens{status: http.StatusBadRequest, body: `{"error":"invalid_grant","error_description":"Refresh token revoked"}`}
	e, client := endpoint(t, s)
	_, err := e.Refresh(context.Background(), client, "client123", "old")
	if !errors.Is(err, ErrReauthorize) {
		t.Errorf("err = %v, want ErrReauthorize", err)
	}
}

// Anything else is worth trying again, and must not be mistaken for the end.
func TestOtherFailuresAreNotTheEnd(t *testing.T) {
	for name, s := range map[string]*tokens{
		"a server error": {status: http.StatusBadGateway, body: `<html>bad gateway</html>`},
		"a rate limit":   {status: http.StatusTooManyRequests, body: `{"error":"rate_limited"}`},
		"no token":       {body: `{"expires_in":3600}`},
	} {
		t.Run(name, func(t *testing.T) {
			e, client := endpoint(t, s)
			_, err := e.Refresh(context.Background(), client, "client123", "RT")
			if err == nil || errors.Is(err, ErrReauthorize) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

// An error must never carry what was sent.
func TestErrorsDoNotQuoteTheRefreshToken(t *testing.T) {
	s := &tokens{status: http.StatusBadRequest, body: `{"error":"invalid_request","error_description":"bad refresh_token SECRET-RT"}`}
	e, client := endpoint(t, s)
	_, err := e.Refresh(context.Background(), client, "client123", "SECRET-RT")
	if err == nil || strings.Contains(err.Error(), "SECRET-RT") {
		t.Errorf("err = %v", err)
	}
}

func TestTheTokenFileRoundTripsAndIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "spotify.json")
	want := Token{ClientID: "client123", RefreshToken: "RT", AuthorizedAt: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != want {
		t.Errorf("loaded %+v, %v", got, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600: it holds the refresh token", info.Mode().Perm())
	}
	// No temporary file is left beside it.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("%d files in the directory, want just the token", len(entries))
	}

	if got.Expires() != want.AuthorizedAt.Add(180*24*time.Hour) {
		t.Errorf("expires %s", got.Expires())
	}
}

func TestLoadRefusesAFileWithNoAuthorization(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"empty.json":     `{}`,
		"no-token.json":  `{"client_id":"c"}`,
		"no-client.json": `{"refresh_token":"r"}`,
		"garbage.json":   `not json`,
	} {
		path := filepath.Join(dir, name)
		_ = os.WriteFile(path, []byte(content), 0o600)
		if _, err := Load(path); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if _, err := Load(filepath.Join(dir, "missing.json")); !os.IsNotExist(err) {
		t.Errorf("a missing file gave %v", err)
	}
}

func source(t *testing.T, s *tokens) (*Source, string, *time.Time) {
	t.Helper()
	e, client := endpoint(t, s)
	path := filepath.Join(t.TempDir(), "spotify.json")
	if err := Save(path, Token{ClientID: "client123", RefreshToken: "RT-1", AuthorizedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	src, err := NewSource(path, client)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	src.Endpoints, src.Now = e, func() time.Time { return now }
	return src, path, &now
}

func TestAnAccessTokenIsReusedUntilItIsNearlySpent(t *testing.T) {
	s := &tokens{body: `{"access_token":"AT-1","expires_in":3600}`}
	src, _, now := source(t, s)

	for i := 0; i < 5; i++ {
		if got, err := src.Access(context.Background()); err != nil || got != "AT-1" {
			t.Fatalf("access = %q, %v", got, err)
		}
		*now = now.Add(10 * time.Minute)
	}
	if len(s.forms) != 1 {
		t.Errorf("refreshed %d times in 50 minutes, want once", len(s.forms))
	}

	*now = now.Add(15 * time.Minute) // 65 minutes in: past the hour
	s.body = `{"access_token":"AT-2","expires_in":3600}`
	if got, _ := src.Access(context.Background()); got != "AT-2" {
		t.Errorf("access after the hour = %q", got)
	}
}

// Spotify may replace the refresh token. The one it replaced stops working,
// so the new one has to be on disk before anything else happens.
func TestAReplacedRefreshTokenIsWrittenBack(t *testing.T) {
	s := &tokens{body: `{"access_token":"AT","refresh_token":"RT-2","expires_in":3600}`}
	src, path, _ := source(t, s)
	authorized := src.Token().AuthorizedAt

	if _, err := src.Access(context.Background()); err != nil {
		t.Fatal(err)
	}
	onDisk, err := Load(path)
	if err != nil || onDisk.RefreshToken != "RT-2" {
		t.Fatalf("on disk: %+v, %v", onDisk, err)
	}
	// A new refresh token is not a new authorization: the six months still
	// run from when a person last agreed.
	if !onDisk.AuthorizedAt.Equal(authorized) {
		t.Error("replacing the token reset the date it was authorized")
	}

	src.Invalidate()
	_, _ = src.Access(context.Background())
	if got := s.forms[1].Get("refresh_token"); got != "RT-2" {
		t.Errorf("the next refresh used %q, want the replacement", got)
	}
}

// Two runs of the agent can hold one authorization: the service, and a person
// listing the account's devices. Whichever refreshes second must use what the
// first was given, not what it read when it started.
func TestATokenReplacedByAnotherRunIsPickedUp(t *testing.T) {
	s := &tokens{body: `{"access_token":"AT","expires_in":3600}`}
	src, path, _ := source(t, s)

	other := src.Token()
	other.RefreshToken = "RT-from-the-other-run"
	if err := Save(path, other); err != nil {
		t.Fatal(err)
	}

	if _, err := src.Access(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.forms[0].Get("refresh_token"); got != "RT-from-the-other-run" {
		t.Fatalf("refreshed with %q, which Spotify would have refused", got)
	}

	// A file for some other app is not this authorization, and is left alone.
	stranger := other
	stranger.ClientID, stranger.RefreshToken = "another-app", "RT-stranger"
	if err := Save(path, stranger); err != nil {
		t.Fatal(err)
	}
	src.Invalidate()
	_, _ = src.Access(context.Background())
	if got := s.forms[1].Get("refresh_token"); got != "RT-from-the-other-run" {
		t.Fatalf("refreshed with %q", got)
	}
}

func TestWithNoReplacementTheOldRefreshTokenStands(t *testing.T) {
	s := &tokens{body: `{"access_token":"AT","expires_in":3600}`}
	src, path, _ := source(t, s)
	_, _ = src.Access(context.Background())
	if onDisk, _ := Load(path); onDisk.RefreshToken != "RT-1" {
		t.Errorf("on disk: %q", onDisk.RefreshToken)
	}
}

func TestInvalidateForcesARefresh(t *testing.T) {
	s := &tokens{body: `{"access_token":"AT","expires_in":3600}`}
	src, _, _ := source(t, s)
	_, _ = src.Access(context.Background())
	src.Invalidate()
	_, _ = src.Access(context.Background())
	if len(s.forms) != 2 {
		t.Errorf("refreshed %d times, want 2", len(s.forms))
	}
}

// --- the callback ---------------------------------------------------------------

func callback(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestACallbackWithTheWrongStateIsRefusedAndChangesNothing(t *testing.T) {
	done := make(chan Callback, 4)
	h := CallbackHandler("the-state", done)

	for _, target := range []string{
		"/callback?code=stolen&state=another-state",
		"/callback?code=stolen",
		"/callback?error=access_denied&state=another-state",
		"/elsewhere?code=stolen&state=the-state",
	} {
		if rec := callback(t, h, target); rec.Code < 400 {
			t.Fatalf("%s was answered %d", target, rec.Code)
		}
	}
	if len(done) != 0 {
		t.Fatalf("a refused request ended the wait: %+v", <-done)
	}

	// And the real one still works afterward.
	if rec := callback(t, h, "/callback?code=the-code&state=the-state"); rec.Code != http.StatusOK {
		t.Fatalf("the real callback was answered %d", rec.Code)
	}
	if got := <-done; got.Code != "the-code" || got.Err != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestACallbackIsAnsweredOnce(t *testing.T) {
	done := make(chan Callback, 4)
	h := CallbackHandler("the-state", done)
	callback(t, h, "/callback?code=first&state=the-state")
	if rec := callback(t, h, "/callback?code=second&state=the-state"); rec.Code < 400 {
		t.Fatalf("a second answer was accepted: %d", rec.Code)
	}
	if got := <-done; got.Code != "first" || len(done) != 0 {
		t.Fatalf("got %+v with %d more waiting", got, len(done))
	}
}

func TestARefusalInTheBrowserIsReported(t *testing.T) {
	done := make(chan Callback, 1)
	rec := callback(t, CallbackHandler("the-state", done), "/callback?error=access_denied&state=the-state")
	got := <-done
	if got.Err == nil || got.Code != "" || !strings.Contains(got.Err.Error(), "access_denied") {
		t.Fatalf("got %+v", got)
	}
	// What the request said is not written into the page.
	if strings.Contains(rec.Body.String(), "access_denied") {
		t.Fatalf("the page repeats the request: %q", rec.Body.String())
	}
}
