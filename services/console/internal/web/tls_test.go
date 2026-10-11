package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// issue makes a root, an issuing CA and a leaf, and writes the leaf with its
// chain, and its key, as a deployment would.
func issue(t *testing.T, cn string, names []string, notBefore, notAfter time.Time) (certFile, keyFile string) {
	t.Helper()
	key := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	sign := func(tmpl, parent *x509.Certificate, pub *ecdsa.PublicKey, signer *ecdsa.PrivateKey) (*x509.Certificate, []byte) {
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
		if err != nil {
			t.Fatal(err)
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return c, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	ca := func(cn string, serial int64) *x509.Certificate {
		return &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
			IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	}

	rootKey, caKey, leafKey := key(), key(), key()
	rootTmpl := ca("test root", 1)
	root, _ := sign(rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	issuing, issuingPEM := sign(ca("test issuing CA", 2), root, &caKey.PublicKey, rootKey)
	_, leafPEM := sign(&x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: cn},
		DNSNames: names, NotBefore: notBefore, NotAfter: notAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, issuing, &leafKey.PublicKey, caKey)

	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, append(leafPEM, issuingPEM...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func TestAnIdentityIsReadWithItsNameAndItsChain(t *testing.T) {
	now := time.Now()
	cert, key := issue(t, "console.eds.mobile.deevnet.net",
		[]string{"www.eds.mobile.deevnet.net", "console.eds.mobile.deevnet.net"}, now.Add(-time.Hour), now.Add(300*24*time.Hour))

	id, err := LoadIdentity(cert, key, now)
	if err != nil {
		t.Fatal(err)
	}
	// The common name, when it is one of the names, not merely the first listed.
	if id.Name != "console.eds.mobile.deevnet.net" || len(id.Names) != 2 {
		t.Errorf("name %q of %v", id.Name, id.Names)
	}
	if id.Chain != 2 {
		t.Errorf("%d certificates would be presented, want the leaf and its CA", id.Chain)
	}
	if cfg := id.TLSConfig(); len(cfg.Certificates) != 1 || cfg.MinVersion < 0x0303 {
		t.Errorf("config %+v", cfg)
	}
}

func TestACommonNameThatIsNotAmongTheNamesIsNotUsed(t *testing.T) {
	now := time.Now()
	cert, key := issue(t, "something else", []string{"console.eds.mobile.deevnet.net"}, now.Add(-time.Hour), now.Add(time.Hour))
	id, err := LoadIdentity(cert, key, now)
	if err != nil || id.Name != "console.eds.mobile.deevnet.net" {
		t.Fatalf("name %q, %v", id.Name, err)
	}
}

func TestAnIdentityTheConsoleCouldNotServeWithIsRefused(t *testing.T) {
	now := time.Now()
	good := []string{"console.eds.mobile.deevnet.net"}

	expiredCert, expiredKey := issue(t, good[0], good, now.Add(-48*time.Hour), now.Add(-time.Hour))
	if _, err := LoadIdentity(expiredCert, expiredKey, now); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("an expired certificate: %v", err)
	}
	earlyCert, earlyKey := issue(t, good[0], good, now.Add(time.Hour), now.Add(48*time.Hour))
	if _, err := LoadIdentity(earlyCert, earlyKey, now); err == nil || !strings.Contains(err.Error(), "not valid until") {
		t.Errorf("a certificate from the future: %v", err)
	}
	namelessCert, namelessKey := issue(t, good[0], nil, now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := LoadIdentity(namelessCert, namelessKey, now); err == nil || !strings.Contains(err.Error(), "names no DNS name") {
		t.Errorf("a certificate with no names: %v", err)
	}

	// A key that is not the certificate's.
	cert, _ := issue(t, good[0], good, now.Add(-time.Hour), now.Add(time.Hour))
	_, otherKey := issue(t, good[0], good, now.Add(-time.Hour), now.Add(time.Hour))
	if _, err := LoadIdentity(cert, otherKey, now); err == nil {
		t.Error("a certificate was taken with someone else's key")
	}
	if _, err := LoadIdentity(filepath.Join(t.TempDir(), "missing.pem"), otherKey, now); err == nil {
		t.Error("a missing certificate was taken")
	}
}
