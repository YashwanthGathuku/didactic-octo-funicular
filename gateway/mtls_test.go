package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// DeriveMTLSState derives mutual TLS verification state strictly from Go runtime
// crypto/tls transport state.
//
// Invariant: mTLSVerified is TRUE if and only if the TLS handshake completed with
// at least one verified certificate chain anchored in the trusted CA pool.
// Headers (e.g. X-Forwarded-Client-Cert) and unverified peer certificates CANNOT
// produce mTLSVerified=true.
func DeriveMTLSState(r *http.Request) (mTLSVerified bool, clientCommonName string) {
	if r.TLS == nil {
		return false, ""
	}
	if len(r.TLS.VerifiedChains) == 0 {
		return false, ""
	}
	chain := r.TLS.VerifiedChains[0]
	if len(chain) == 0 {
		return false, ""
	}
	leaf := chain[0]
	if leaf == nil {
		return false, ""
	}
	return true, leaf.Subject.CommonName
}

func TestMTLSVerification_PlainHTTPCannotProduceVerifiedState(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://gateway.example.com/api/v1/edge/sync", nil)
	// Attempt header forgery
	req.Header.Set("X-Forwarded-Client-Cert", `Subject="CN=EDGE-AGENT-01"`)
	req.Header.Set("X-Client-Verified", "SUCCESS")

	verified, cn := DeriveMTLSState(req)
	if verified {
		t.Error("plain HTTP request must never produce mTLSVerified=true")
	}
	if cn != "" {
		t.Errorf("expected empty common name, got %q", cn)
	}
}

func TestMTLSVerification_TLSWithoutClientCertCannotProduceVerifiedState(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://gateway.example.com/api/v1/edge/sync", nil)
	req.TLS = &tls.ConnectionState{
		HandshakeComplete: true,
		VerifiedChains:    nil, // No client cert verified
	}

	verified, _ := DeriveMTLSState(req)
	if verified {
		t.Error("TLS connection without client certificate must produce mTLSVerified=false")
	}
}

func TestMTLSVerification_UntrustedClientCertCannotProduceVerifiedState(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://gateway.example.com/api/v1/edge/sync", nil)
	// Untrusted peer certificate presented, but VerifiedChains is empty because it failed CA validation
	untrustedCert := &x509.Certificate{
		Subject: pkix.Name{CommonName: "ATTACKER-AGENT"},
	}
	req.TLS = &tls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{untrustedCert},
		VerifiedChains:    nil, // Failed CA verification
	}

	verified, cn := DeriveMTLSState(req)
	if verified {
		t.Error("untrusted client certificate must produce mTLSVerified=false")
	}
	if cn != "" {
		t.Errorf("untrusted cert should not extract CN, got %q", cn)
	}
}

func TestMTLSVerification_RealVerifiedClientCertificateProducesTrue(t *testing.T) {
	// Generate in-memory CA and client cert
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "SentinelFlow Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "EDGE-AGENT-MERIDIAN-VPC-01"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, _ := x509.ParseCertificate(clientDER)

	req := httptest.NewRequest(http.MethodPost, "https://gateway.example.com/api/v1/edge/sync", nil)
	req.TLS = &tls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{clientCert},
		VerifiedChains:    [][]*x509.Certificate{{clientCert, caCert}},
	}

	verified, cn := DeriveMTLSState(req)
	if !verified {
		t.Error("verified client certificate chain must produce mTLSVerified=true")
	}
	if cn != "EDGE-AGENT-MERIDIAN-VPC-01" {
		t.Errorf("expected CN 'EDGE-AGENT-MERIDIAN-VPC-01', got %q", cn)
	}
}
