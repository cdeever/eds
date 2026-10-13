package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/spotify"
	"github.com/cdeever/eds/services/nowplaying/internal/spotifyauth"
)

// spotifyAuth has the account's owner authorize the agent, in a browser on
// this machine, and keeps the result.
//
// It has to run where there is a browser: Spotify sends the browser back to
// 127.0.0.1, which is whichever machine the browser is on. A Pi has none, so
// this is run on a desk and the file it writes is carried over.
func spotifyAuth(ctx context.Context) error {
	clientID := strings.TrimSpace(os.Getenv("NP_SPOTIFY_CLIENT_ID"))
	if clientID == "" {
		return errors.New("NP_SPOTIFY_CLIENT_ID is not set: it is the Client ID of the app at developer.spotify.com")
	}
	path, err := spotifyTokenPath()
	if err != nil {
		return err
	}

	verifier, err := spotifyauth.NewVerifier()
	if err != nil {
		return err
	}
	state, err := spotifyauth.NewState()
	if err != nil {
		return err
	}

	// The loopback address only: nothing else on the network can reach this,
	// and it is open for as long as the authorization takes.
	redirect, _ := url.Parse(spotifyauth.RedirectURI)
	listener, err := net.Listen("tcp4", redirect.Host)
	if err != nil {
		return fmt.Errorf("could not listen on %s, where Spotify sends the browser back: %w", redirect.Host, err)
	}
	defer listener.Close()

	done := make(chan spotifyauth.Callback, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: spotifyauth.CallbackHandler(state, done)}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	page := spotifyauth.Spotify.AuthorizeURL(clientID, state, spotifyauth.Challenge(verifier))
	fmt.Println("Asking Spotify for permission to see what this account is playing, and to")
	fmt.Println("play, pause, skip and set the volume. It asks for nothing else:", strings.Join(spotifyauth.Scopes, ", "))
	fmt.Println()
	fmt.Println("A browser should open. If it does not, open this:")
	fmt.Println()
	fmt.Println("  " + page)
	fmt.Println()
	fmt.Println("Waiting up to five minutes for Agree to be clicked. Ctrl-C to give up.")
	if config.Str("NP_SPOTIFY_NO_BROWSER", "") == "" {
		openBrowser(page)
	}

	var answer spotifyauth.Callback
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		return errors.New("nobody answered in the browser")
	case answer = <-done:
	}
	if answer.Err != nil {
		return answer.Err
	}

	exchange, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	grant, err := spotifyauth.Spotify.Exchange(exchange, spotifyHTTP(), clientID, answer.Code, verifier)
	if err != nil {
		return err
	}
	if grant.RefreshToken == "" {
		return errors.New("spotify answered without a refresh token, so there is nothing to keep")
	}

	token := spotifyauth.Token{ClientID: clientID, RefreshToken: grant.RefreshToken, AuthorizedAt: time.Now().UTC()}
	if err := spotifyauth.Save(path, token); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Authorized. Kept in", path)
	fmt.Printf("It lasts until %s, whatever is done with it; then this is run again.\n", token.Expires().Format("2 January 2006"))
	fmt.Println()
	fmt.Println("Next: npagent spotify-devices")
	return nil
}

func openBrowser(page string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", page)
	case "linux":
		cmd = exec.Command("xdg-open", page)
	default:
		return
	}
	// If it fails the address has been printed, which is the fallback.
	_ = cmd.Start()
}

// spotifyDevices lists the account's devices, so that the ones that count
// can be named exactly as Spotify names them.
func spotifyDevices(ctx context.Context) error {
	path, err := spotifyTokenPath()
	if err != nil {
		return err
	}
	tokens, err := spotifyauth.NewSource(path, spotifyHTTP())
	if os.IsNotExist(err) {
		return errors.New("not authorized yet: run `npagent spotify-auth` first")
	}
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	access, err := tokens.Access(ctx)
	if err != nil {
		return err
	}
	devices, err := spotify.ListDevices(ctx, spotifyHTTP(), "", access)
	if err != nil {
		return err
	}

	named := map[string]bool{}
	for _, name := range config.List("NP_SPOTIFY_DEVICES", "") {
		named[strings.ToLower(name)] = true
	}

	if len(devices) == 0 {
		fmt.Println("Spotify lists no devices. It lists a device only while Spotify is open on it:")
		fmt.Println("open Spotify where playback should count, and run this again.")
		return nil
	}
	fmt.Println("The account's devices, as Spotify names them:")
	fmt.Println()
	for _, d := range devices {
		note := ""
		if d.Active {
			note = "  (active now)"
		}
		if named[strings.ToLower(strings.TrimSpace(d.Name))] {
			note += "  <- counts"
		}
		fmt.Printf("  %-32s %s%s\n", d.Name, d.Type, note)
	}
	fmt.Println()
	fmt.Println("Only devices open right now are listed. Name the ones that count in")
	fmt.Println("NP_SPOTIFY_DEVICES, separated by commas.")
	return nil
}
