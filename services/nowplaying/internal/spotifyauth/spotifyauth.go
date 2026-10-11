// Package spotifyauth is how the agent is allowed to ask Spotify what an
// account is playing (ADR-0012).
//
// It uses the authorization-code flow with PKCE, which needs no client
// secret: there is nothing here to keep but a refresh token, in a file that
// only the agent can read.
//
// It asks for permission to read playback and nothing more. Controlling
// playback is a further permission, and is not asked for until something
// uses it.
package spotifyauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scopes are what the agent asks to be allowed to do: see what is playing,
// and on which device.
var Scopes = []string{"user-read-playback-state", "user-read-currently-playing"}

// Lifetime is how long an authorization lasts. Since mid-2026 Spotify ends
// one six months after it was granted, and refreshing does not extend it.
const Lifetime = 180 * 24 * time.Hour

// RedirectURI is where Spotify sends the browser back. It must be registered
// on the app exactly, and must name the loopback address by number: Spotify
// accepts plain http for 127.0.0.1 and refuses "localhost".
const RedirectURI = "http://127.0.0.1:8898/callback"

// ErrReauthorize means the authorization has ended - expired, or withdrawn by
// the account's owner. Nothing the agent can do renews it; a person has to
// authorize again.
var ErrReauthorize = errors.New("spotify: the authorization has ended; authorize again")

// Endpoints are Spotify's, as fields so tests can stand in for them.
type Endpoints struct {
	Authorize string // https://accounts.spotify.com/authorize
	Token     string // https://accounts.spotify.com/api/token
}

// Spotify is where the real ones are.
var Spotify = Endpoints{
	Authorize: "https://accounts.spotify.com/authorize",
	Token:     "https://accounts.spotify.com/api/token",
}

// --- PKCE ------------------------------------------------------------------------

// NewVerifier makes the secret half of a PKCE pair.
func NewVerifier() (string, error) {
	return randomString(64)
}

// NewState makes the value that ties a callback to the request that began it.
func NewState() (string, error) {
	return randomString(24)
}

func randomString(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// Challenge is the public half: the SHA-256 of the verifier, base64url
// without padding (RFC 7636, the S256 method).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL is the page the account's owner is sent to.
func (e Endpoints) AuthorizeURL(clientID, state, challenge string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {RedirectURI},
		"scope":                 {strings.Join(Scopes, " ")},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {challenge},
	}
	return e.Authorize + "?" + q.Encode()
}

// --- tokens ----------------------------------------------------------------------

// Grant is what Spotify hands back: a short-lived access token, and sometimes
// a refresh token to replace the one that was used.
type Grant struct {
	AccessToken  string
	RefreshToken string // empty when Spotify sent none; the old one still stands
	ExpiresIn    time.Duration
}

// Exchange turns the code from the callback into the first grant.
func (e Endpoints) Exchange(ctx context.Context, client *http.Client, clientID, code, verifier string) (Grant, error) {
	return e.token(ctx, client, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {RedirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
}

// Refresh gets a new access token. It returns ErrReauthorize when Spotify
// says the refresh token is no longer good, which is not worth retrying.
func (e Endpoints) Refresh(ctx context.Context, client *http.Client, clientID, refreshToken string) (Grant, error) {
	return e.token(ctx, client, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	})
}

func (e Endpoints) token(ctx context.Context, client *http.Client, form url.Values) (Grant, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return Grant{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return Grant{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Grant{}, err
	}

	var answer struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.Unmarshal(body, &answer)

	if resp.StatusCode != http.StatusOK {
		if answer.Error == "invalid_grant" {
			return Grant{}, fmt.Errorf("%w (%s)", ErrReauthorize, answer.Description)
		}
		// Never the body: on some failures it echoes what was sent.
		return Grant{}, fmt.Errorf("spotify: the token endpoint answered %s (%s)", resp.Status, answer.Error)
	}
	if answer.AccessToken == "" {
		return Grant{}, errors.New("spotify: the token endpoint answered without an access token")
	}
	return Grant{
		AccessToken:  answer.AccessToken,
		RefreshToken: answer.RefreshToken,
		ExpiresIn:    time.Duration(answer.ExpiresIn) * time.Second,
	}, nil
}

// --- the token file --------------------------------------------------------------

// Token is what is kept between runs. The client id is not a secret under
// PKCE, but it travels with the token it belongs to, so that the file is all
// a driver needs and nothing about the app has to be configured twice.
type Token struct {
	ClientID     string    `json:"client_id"`
	RefreshToken string    `json:"refresh_token"`
	AuthorizedAt time.Time `json:"authorized_at"`
}

// Expires is when the authorization ends, whatever is done to it.
func (t Token) Expires() time.Time {
	return t.AuthorizedAt.Add(Lifetime)
}

// Load reads a token file.
func Load(path string) (Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Token{}, err
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return Token{}, fmt.Errorf("%s: %w", path, err)
	}
	if t.ClientID == "" || t.RefreshToken == "" {
		return Token{}, fmt.Errorf("%s holds no authorization", path)
	}
	return t, nil
}

// Save writes a token file so that a reader sees the old one or the new one
// and never half of either: Spotify may replace the refresh token on any
// refresh, and the one it replaced stops working. A crash between the two
// must not leave neither.
func Save(path string, t Token) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".spotify-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// --- an access token, kept fresh ---------------------------------------------------

// Source hands out an access token, refreshing it when it is close to
// expiring and writing back a refresh token Spotify replaced.
type Source struct {
	Endpoints Endpoints
	HTTP      *http.Client
	Path      string
	Now       func() time.Time

	token   Token
	access  string
	expires time.Time
}

// NewSource loads the token file.
func NewSource(path string, client *http.Client) (*Source, error) {
	t, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Source{Endpoints: Spotify, HTTP: client, Path: path, Now: time.Now, token: t}, nil
}

// Token is the authorization as last read or written.
func (s *Source) Token() Token { return s.token }

// Access returns a token good for at least the next minute.
func (s *Source) Access(ctx context.Context) (string, error) {
	if s.access != "" && s.Now().Add(time.Minute).Before(s.expires) {
		return s.access, nil
	}
	return s.refresh(ctx)
}

// Invalidate discards the access token, for when the API has refused it.
func (s *Source) Invalidate() { s.access = "" }

func (s *Source) refresh(ctx context.Context) (string, error) {
	grant, err := s.Endpoints.Refresh(ctx, s.HTTP, s.token.ClientID, s.token.RefreshToken)
	if err != nil {
		return "", err
	}

	if grant.RefreshToken != "" && grant.RefreshToken != s.token.RefreshToken {
		// The old refresh token is finished as of now. If this cannot be
		// written, the access token in hand is the last this run will get.
		replaced := s.token
		replaced.RefreshToken = grant.RefreshToken
		if err := Save(s.Path, replaced); err != nil {
			return "", fmt.Errorf("spotify: the refresh token was replaced and could not be saved: %w", err)
		}
		s.token = replaced
	}

	s.access = grant.AccessToken
	s.expires = s.Now().Add(grant.ExpiresIn)
	return s.access, nil
}
