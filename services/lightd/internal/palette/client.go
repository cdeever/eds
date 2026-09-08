// Package palette is the client for the palette extraction service.
//
// The boundary between them is an HTTP call rather than a library call because
// they are different languages for good reasons: the image mathematics lives
// where the ecosystem is, and the long-running daemon is a static binary.
package palette

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ContractVersion is the `v` this client understands. The palette service
// stamps every response; a bump means the shape changed and guessing would be
// worse than refusing.
const ContractVersion = 1

// Swatch mirrors one entry of the palette service's response. It is
// deliberately raw - no role assignment, no LED correction - so that the
// decisions belonging to hardware stay on this side.
type Swatch struct {
	RGB       [3]int     `json:"rgb"`
	Hex       string     `json:"hex"`
	OKLab     [3]float64 `json:"oklab"`
	Weight    float64    `json:"weight"`
	Chroma    float64    `json:"chroma"`
	Lightness float64    `json:"lightness"`
}

// Source describes the image the palette was taken from.
type Source struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
}

// Response is the whole contract.
type Response struct {
	V                 int      `json:"v"`
	Swatches          []Swatch `json:"swatches"`
	BackgroundDropped float64  `json:"background_dropped"`
	Fallback          bool     `json:"fallback"`
	Source            Source   `json:"source"`
}

// StatusError is returned when the palette service answers with a non-200.
// It carries the status so callers can tell "you sent me a bad image" apart
// from "I am broken" - the first is the caller's mistake, the second is not,
// and collapsing them into one error makes a client's own 400 look like a
// server fault.
type StatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("palette: %s: %s", e.Status, e.Body)
}

// ClientFault reports whether the palette service blamed the request.
func (e *StatusError) ClientFault() bool {
	return e.StatusCode >= 400 && e.StatusCode < 500
}

// Client calls the palette service.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client with a timeout short enough that a wedged palette
// service surfaces as an error rather than a hung request holding the stand
// on its previous colour indefinitely.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Extract posts image bytes and returns the ranked palette.
func (c *Client) Extract(ctx context.Context, image []byte, swatches int) (*Response, error) {
	if len(image) == 0 {
		return nil, fmt.Errorf("palette: no image supplied")
	}

	endpoint, err := url.JoinPath(c.BaseURL, "/v1/palette")
	if err != nil {
		return nil, fmt.Errorf("palette: bad base url %q: %w", c.BaseURL, err)
	}
	if swatches > 0 {
		endpoint += "?n=" + strconv.Itoa(swatches)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(image))
	if err != nil {
		return nil, fmt.Errorf("palette: build request: %w", err)
	}
	// Raw body rather than multipart: there is no filename worth sending and
	// no reason to make the service parse an envelope.
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("palette: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("palette: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(bytes.TrimSpace(body)),
		}
	}

	var parsed Response
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("palette: decode response: %w", err)
	}
	if parsed.V != ContractVersion {
		return nil, fmt.Errorf("palette: contract v%d, want v%d", parsed.V, ContractVersion)
	}
	if len(parsed.Swatches) == 0 {
		return nil, fmt.Errorf("palette: response contained no swatches")
	}
	return &parsed, nil
}
