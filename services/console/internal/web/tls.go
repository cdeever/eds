package web

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Identity is the certificate the console serves with, and what it says.
type Identity struct {
	Certificate tls.Certificate
	// Name is the DNS name the certificate is for: its common name when that
	// is one of its names, else the first of them. It is where the plain
	// port sends people.
	Name     string
	Names    []string
	NotAfter time.Time
	// Chain is how many certificates are presented: the leaf and the CAs
	// after it. A client that holds only the root needs every CA between.
	Chain int
}

// LoadIdentity reads a certificate with its chain, and its key, and refuses
// one the console could not usefully serve with: one that has expired, that
// is not yet valid, or that names nothing.
//
// It is read once, at start. A renewed certificate is put in place and the
// console restarted, which the deployment does.
func LoadIdentity(certFile, keyFile string, now time.Time) (Identity, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return Identity{}, fmt.Errorf("the certificate and key could not be read together: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return Identity{}, fmt.Errorf("the certificate could not be read: %w", err)
	}
	pair.Leaf = leaf

	switch {
	case len(leaf.DNSNames) == 0:
		return Identity{}, errors.New("the certificate names no DNS name, so no browser would accept it")
	case now.After(leaf.NotAfter):
		return Identity{}, fmt.Errorf("the certificate expired on %s", leaf.NotAfter.UTC().Format("2006-01-02"))
	case now.Before(leaf.NotBefore):
		return Identity{}, fmt.Errorf("the certificate is not valid until %s; is the clock right?", leaf.NotBefore.UTC().Format(time.RFC3339))
	}

	name := leaf.DNSNames[0]
	if slices.Contains(leaf.DNSNames, leaf.Subject.CommonName) {
		name = leaf.Subject.CommonName
	}
	return Identity{
		Certificate: pair, Name: name, Names: leaf.DNSNames,
		NotAfter: leaf.NotAfter, Chain: len(pair.Certificate),
	}, nil
}

// TLSConfig is how the console serves: its one certificate, TLS 1.2 or later.
func (id Identity) TLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{id.Certificate},
		MinVersion:   tls.VersionTLS12,
	}
}
