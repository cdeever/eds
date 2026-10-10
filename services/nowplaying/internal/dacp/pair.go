package dacp

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Pairing is this remote's identity: the GUID a player remembers it by.
//
// It is the only state the client keeps. Lose it and the player no longer
// knows this remote, and the code has to be typed in again.
type Pairing struct {
	GUID string `json:"guid"` // sixteen hex digits, upper case
	Host string `json:"host"` // where the player answered from
	Port int    `json:"port"`
	Name string `json:"name"` // what the player showed during pairing
}

// NewGUID makes a pairing GUID.
func NewGUID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(raw[:])), nil
}

// NewPIN makes the four-digit code the user types into the player.
func NewPIN() (string, error) {
	var raw [2]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d", (int(raw[0])<<8|int(raw[1]))%10000), nil
}

// PairingCode is what the player sends back to prove the user typed the PIN
// this remote displayed: the MD5 of the advertised GUID followed by each
// digit of the PIN as a two-byte character.
func PairingCode(guid, pin string) string {
	h := md5.New()
	h.Write([]byte(guid))
	for i := 0; i < len(pin); i++ {
		h.Write([]byte{pin[i], 0})
	}
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

// TXT is what a remote advertises under _touch-remote._tcp while it waits to
// be paired. The player lists the remote by DvNm, and computes its side of
// the pairing code from Pair.
func TXT(guid, name string) map[string]string {
	return map[string]string{
		"DvNm":    name,
		"RemV":    "10000",
		"DvTy":    "iPod",
		"RemN":    "Remote",
		"txtvers": "1",
		"Pair":    guid,
	}
}

// Paired is what a successful pairing reports.
type Paired struct {
	Host string // the player's address, as it called back from
}

// PairHandler answers the player's call back. After the user types the PIN,
// the player requests /pair with the code it computed; a remote that agrees
// answers with its GUID, and the player stores it.
//
// done receives once, when a pairing with the right code arrives.
func PairHandler(guid, pin, name string, done chan<- Paired) http.Handler {
	want := PairingCode(guid, pin)
	guidValue, _ := strconv.ParseUint(guid, 16, 64)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pair" {
			http.NotFound(w, r)
			return
		}
		got := strings.ToUpper(r.URL.Query().Get("pairingcode"))
		if got != want {
			// A wrong PIN, or someone else's remote being paired at the same
			// moment. The player shows the user an error; nothing is stored.
			http.Error(w, "wrong pairing code", http.StatusNotFound)
			return
		}

		body := append(EncodeUint64("cmpg", guidValue), Encode("cmnm", []byte(name))...)
		body = append(body, Encode("cmty", []byte("iPod"))...)
		w.Header().Set("Content-Type", "application/x-dmap-tagged")
		_, _ = w.Write(Encode("cmpa", body))

		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		select {
		case done <- Paired{Host: host}:
		default:
		}
	})
}
