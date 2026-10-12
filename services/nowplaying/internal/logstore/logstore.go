// Package logstore writes lines to the tenant's log store from a workload
// (ADR-0027).
//
// A device's lines go by MQTT and a bridge (ADR-0005). A workload is not a
// device and has no such route: it is given an endpoint and a token that
// writes to the tenant's workload partition, and posts there itself. That is
// the substrate's arrangement (Deevnet ADR-0027), and this is the whole of
// EdS's side of it.
//
// It is not the service's log. The journal has that. This carries the few
// lines worth a graph.
package logstore

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Writer posts lines to the store.
type Writer struct {
	// Endpoint is the store's address, as the tenant is given it.
	Endpoint string
	// Token writes to the tenant's workload partition and reads nothing. It
	// is never logged.
	Token string
	// App is who is writing: every line carries it.
	App  string
	HTTP *http.Client
}

// New makes a Writer that trusts the certificate authority in caFile, which
// is the site's own. An empty caFile trusts what the system does.
func New(endpoint, token, app, caFile string) (*Writer, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("logstore: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("logstore: no certificate in %s", caFile)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &Writer{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Token:    token,
		App:      app,
		HTTP:     &http.Client{Timeout: 10 * time.Second, Transport: transport},
	}, nil
}

// Write stores one line: a message and its fields. The store dates it on
// arrival.
func (w *Writer) Write(ctx context.Context, msg string, fields map[string]any) error {
	line := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		line[k] = v
	}
	// The store's own names win over a field that happens to share one.
	line["_msg"] = msg
	line["app"] = w.App
	body, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("logstore: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.Endpoint+"/insert/jsonline", bytes.NewReader(append(body, '\n')))
	if err != nil {
		return fmt.Errorf("logstore: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	// Without this the store answers 200 and keeps nothing.
	req.Header.Set("Content-Type", "application/stream+json")

	resp, err := w.HTTP.Do(req)
	if err != nil {
		// The error names the address and never the token.
		return fmt.Errorf("logstore: %w", err)
	}
	defer resp.Body.Close()
	said, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	if resp.StatusCode/100 != 2 {
		return errors.New("logstore: " + resp.Status + ": " + strings.TrimSpace(string(said)))
	}
	return nil
}
