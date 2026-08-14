package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// ---------------------------------------------------------------------------
// SECURITY REMEDIATION 2026-08-14
//
// The previous VerifyDetachedPgpSignature returned IsAuthentic=true for ANY
// input containing "-----BEGIN PGP SIGNATURE-----". It performed no
// cryptographic verification, consulted no keyring, and returned a hardcoded
// key ID. Any party could forge a non-repudiation record over arbitrary bytes.
//
// VerifySshPublicKey returned IsValid=true for anything that base64-decoded and
// reported a hardcoded 4096 bits for every RSA key -- so a weak 1024-bit key
// was recorded in the audit trail as compliant.
//
// Both now perform real verification and FAIL CLOSED.
// ---------------------------------------------------------------------------

type SshKeyVerificationResult struct {
	IsValid     bool      `json:"isValid"`
	KeyType     string    `json:"keyType"`
	Fingerprint string    `json:"fingerprint"`
	KeyBits     int       `json:"keyBits"`
	VerifiedAt  time.Time `json:"verifiedAt"`
	Comment     string    `json:"comment,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

type PgpSignatureVerificationResult struct {
	IsAuthentic  bool      `json:"isAuthentic"`
	SignerKeyId  string    `json:"signerKeyId"`
	DigestAlg    string    `json:"digestAlg"`
	PayloadHash  string    `json:"payloadHash"`
	VerifiedAt   time.Time `json:"verifiedAt"`
	StatusReason string    `json:"statusReason"`
}

// sshWireStrings splits an SSH public key blob into length-prefixed fields per
// RFC 4253 s6.6.
func sshWireStrings(blob []byte) ([][]byte, error) {
	var out [][]byte
	for len(blob) > 0 {
		if len(blob) < 4 {
			return nil, errors.New("truncated length prefix in SSH key blob")
		}
		n := binary.BigEndian.Uint32(blob[:4])
		blob = blob[4:]
		if uint32(len(blob)) < n {
			return nil, errors.New("length prefix exceeds remaining SSH key blob")
		}
		out = append(out, blob[:n])
		blob = blob[n:]
		if len(out) > 16 {
			break
		}
	}
	return out, nil
}

// VerifySshPublicKey parses an OpenSSH public key, verifies the algorithm name
// embedded in the wire blob matches the declared prefix, derives the TRUE key
// size, enforces NIST SP 800-131A Rev.2 minimum RSA strength, and computes the
// standard SHA256 fingerprint (matches `ssh-keygen -lf`).
func VerifySshPublicKey(rawKey string) (*SshKeyVerificationResult, error) {
	now := time.Now().UTC()
	deny := func(reason string, err error) (*SshKeyVerificationResult, error) {
		return &SshKeyVerificationResult{IsValid: false, VerifiedAt: now, Reason: reason}, err
	}

	parts := strings.Fields(strings.TrimSpace(rawKey))
	if len(parts) < 2 {
		return deny("invalid OpenSSH key format: expected '<type> <base64>'", errors.New("invalid OpenSSH key format"))
	}

	declaredType, b64Key := parts[0], parts[1]
	comment := ""
	if len(parts) > 2 {
		comment = strings.Join(parts[2:], " ")
	}

	keyBytes, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return deny("invalid base64 in public key", fmt.Errorf("invalid base64: %w", err))
	}

	fields, err := sshWireStrings(keyBytes)
	if err != nil || len(fields) == 0 {
		return deny("malformed SSH key blob (RFC 4253 wire format)", errors.New("malformed SSH key blob"))
	}

	embeddedType := string(fields[0])
	if embeddedType != declaredType {
		return deny(
			fmt.Sprintf("algorithm mismatch: prefix says %q, key blob says %q", declaredType, embeddedType),
			errors.New("SSH key algorithm mismatch"))
	}

	keyBits := 0
	switch {
	case embeddedType == "ssh-ed25519":
		if len(fields) < 2 || len(fields[1]) != 32 {
			return deny("ed25519 key must carry a 32-byte point", errors.New("bad ed25519 key"))
		}
		keyBits = 256
	case embeddedType == "ssh-rsa":
		if len(fields) < 3 {
			return deny("RSA key missing exponent/modulus", errors.New("bad rsa key"))
		}
		modulus := bytes.TrimLeft(fields[2], "\x00")
		keyBits = len(modulus) * 8
		if keyBits < 2048 {
			return &SshKeyVerificationResult{
				IsValid: false, KeyType: embeddedType, KeyBits: keyBits, VerifiedAt: now,
				Reason: fmt.Sprintf("RSA modulus is %d bits; policy minimum is 2048 (NIST SP 800-131A Rev.2)", keyBits),
			}, errors.New("rsa key below policy minimum")
		}
	case strings.HasPrefix(embeddedType, "ecdsa-sha2-nistp"):
		switch {
		case strings.HasSuffix(embeddedType, "256"):
			keyBits = 256
		case strings.HasSuffix(embeddedType, "384"):
			keyBits = 384
		case strings.HasSuffix(embeddedType, "521"):
			keyBits = 521
		default:
			return deny("unrecognised ECDSA curve", errors.New("unknown ecdsa curve"))
		}
	default:
		return deny(fmt.Sprintf("unsupported key algorithm %q", embeddedType), errors.New("unsupported ssh key algorithm"))
	}

	sum := sha256.Sum256(keyBytes)
	return &SshKeyVerificationResult{
		IsValid:     true,
		KeyType:     embeddedType,
		Fingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:]),
		KeyBits:     keyBits,
		VerifiedAt:  now,
		Comment:     comment,
	}, nil
}

// loadCounterpartyKeyring reads the trusted counterparty public keyring from
// SENTINEL_PGP_KEYRING. No default, no fallback: absent a keyring there is no
// basis on which to assert authenticity, so verification fails closed.
// resetKeyringCacheForTest exists so tests can re-point SENTINEL_PGP_KEYRING.
func resetKeyringCacheForTest() {}

func loadCounterpartyKeyring() (openpgp.EntityList, error) {
	path := os.Getenv("SENTINEL_PGP_KEYRING")
	if path == "" {
		return nil, errors.New("SENTINEL_PGP_KEYRING not configured; refusing to assert authenticity without a trusted keyring")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open PGP keyring at %s: %w", path, err)
	}
	defer f.Close()

	if ring, err := openpgp.ReadArmoredKeyRing(f); err == nil {
		return ring, nil
	}
	if _, seekErr := f.Seek(0, 0); seekErr == nil {
		if ring, err := openpgp.ReadKeyRing(f); err == nil {
			return ring, nil
		}
	}
	return nil, errors.New("cannot parse PGP keyring (tried armored and binary)")
}

// VerifyDetachedPgpSignature performs real detached-signature verification of
// payload against signatureArmored using the configured trusted keyring.
// Fails closed on missing keyring, unknown signer, or signature mismatch.
func VerifyDetachedPgpSignature(payload []byte, signatureArmored string) (*PgpSignatureVerificationResult, error) {
	sum := sha256.Sum256(payload)
	payloadHashHex := hex.EncodeToString(sum[:])

	fail := func(reason string, err error) (*PgpSignatureVerificationResult, error) {
		return &PgpSignatureVerificationResult{
			IsAuthentic:  false,
			PayloadHash:  payloadHashHex,
			VerifiedAt:   time.Now().UTC(),
			StatusReason: reason,
		}, err
	}

	if !strings.Contains(signatureArmored, "-----BEGIN PGP SIGNATURE-----") {
		return fail("Missing PGP armor header (-----BEGIN PGP SIGNATURE-----)", errors.New("malformed PGP armor"))
	}

	keyring, err := loadCounterpartyKeyring()
	if err != nil {
		return fail("Cannot verify: "+err.Error(), err)
	}

	signer, err := openpgp.CheckArmoredDetachedSignature(
		keyring, bytes.NewReader(payload), strings.NewReader(signatureArmored), nil)
	if err != nil {
		return fail("Signature verification FAILED: "+err.Error(), err)
	}
	if signer == nil || signer.PrimaryKey == nil {
		return fail("Signature verification returned no signing entity", errors.New("no signer"))
	}

	return &PgpSignatureVerificationResult{
		IsAuthentic:  true,
		SignerKeyId:  fmt.Sprintf("0x%X", signer.PrimaryKey.KeyId),
		DigestAlg:    "SHA-256 (NIST FIPS 180-4)",
		PayloadHash:  payloadHashHex,
		VerifiedAt:   time.Now().UTC(),
		StatusReason: "Detached signature cryptographically verified against configured counterparty keyring.",
	}, nil
}
