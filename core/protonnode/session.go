// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leggerezza/android-proton/core/proton"
	"github.com/leggerezza/android-proton/core/stealth"
)

// sessionJSON is the credential bundle that the Android app stores encrypted.
// It is the ONLY structure that contains a token or a private key, and it
// never reaches the disk through this package (see store.Store docs).
type sessionJSON struct {
	UID         string              `json:"uid,omitempty"`
	Access      string              `json:"access_token,omitempty"`
	Refresh     string              `json:"refresh_token,omitempty"`
	Email       string              `json:"login_email,omitempty"`
	DeviceName  string              `json:"device_name,omitempty"`
	SeedB64     string              `json:"ed25519_seed,omitempty"`
	Certificate *proton.Certificate `json:"certificate,omitempty"`
	LastServer  string              `json:"last_server,omitempty"`
	UpdatedAt   time.Time           `json:"updated_at,omitempty"`
}

func (s *sessionJSON) tokens() proton.Tokens {
	return proton.Tokens{UID: s.UID, Access: s.Access, Refresh: s.Refresh}
}

func (s *sessionJSON) setTokens(t proton.Tokens) {
	s.UID = t.UID
	s.Access = t.Access
	s.Refresh = t.Refresh
	s.UpdatedAt = time.Now()
}

// keyPair restores the device key pair, generating one on first use. The seed
// is the part worth persisting: it is what Proton registered as a device.
// rotateSeed replaces the Ed25519 seed with a fresh random one; Proton refuses a
// public key whose fingerprint is already registered, and only a new key clears
// that conflict.
func (s *sessionJSON) rotateSeed() (*stealth.KeyPair, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("generate ed25519 seed: %w", err)
	}
	kp, err := stealth.KeyPairFromSeed(seed)
	if err != nil {
		return nil, err
	}
	s.SeedB64 = base64.StdEncoding.EncodeToString(kp.Seed())
	s.Certificate = nil
	return kp, nil
}

func (s *sessionJSON) keyPair() (*stealth.KeyPair, error) {
	if s.SeedB64 == "" {
		kp, err := stealth.NewKeyPair()
		if err != nil {
			return nil, err
		}
		s.SeedB64 = base64.StdEncoding.EncodeToString(kp.Seed())
		if s.DeviceName == "" {
			s.DeviceName = randomDeviceName()
		}
		return kp, nil
	}
	seed, err := base64.StdEncoding.DecodeString(s.SeedB64)
	if err != nil {
		return nil, fmt.Errorf("stored ed25519 seed is corrupt: %w", err)
	}
	return stealth.KeyPairFromSeed(seed)
}

// randomDeviceName is stable for the life of the session and unique enough
// that Proton does not confuse two installs. Android has no hostname, so the
// name is generated once and persisted (PROMPT.md: the same name must not
// create a new device on every launch).
func randomDeviceName() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "protonnode"
	}
	return "protonnode-" + hex.EncodeToString(b[:])
}

// certificateFresh reports whether the cached certificate can still be used.
// Proton issues 7-day certificates and the baseline renews 24h before expiry.
func (s *sessionJSON) certificateFresh() bool {
	if s.Certificate == nil || s.DeviceName == "" {
		return false
	}
	return s.Certificate.Fresh(24*time.Hour) && s.Certificate.DeviceName == s.DeviceName
}

// ensureCertificate requests a new certificate when the cached one is missing,
// stale or registered under another device name.
func (e *engine) ensureCertificate(ctx context.Context) error {
	kp, err := e.session.keyPair()
	if err != nil {
		return err
	}
	if e.session.certificateFresh() {
		e.logfOrNil("certificate: reusing serial %s (expires %s)",
			e.session.Certificate.SerialNumber, e.session.Certificate.ExpiresAt().Format(time.RFC3339))
		return nil
	}
	pubPEM, err := kp.PublicKeyPEM()
	if err != nil {
		return err
	}
	e.logfOrNil("certificate: requesting a new one for device %s", e.session.DeviceName)
	cert, err := e.client.RequestCertificate(ctx, pubPEM, e.session.DeviceName, proton.CertificateFeatures{})
	if err != nil && isFingerprintConflict(err) {
		// Proton keeps a fingerprint per device; reusing a key that is already
		// registered elsewhere fails with 409/2500 ("fingerprint conflict").
		// Rotating the Ed25519 seed is the documented fix, and the rotated seed
		// is persisted with the session on the next poll.
		e.logfOrNil("certificate: key fingerprint conflict, rotating the client key")
		kp, rerr := e.session.rotateSeed()
		if rerr != nil {
			return rerr
		}
		e.markSessionDirty()
		if pubPEM, err = kp.PublicKeyPEM(); err != nil {
			return err
		}
		cert, err = e.client.RequestCertificate(ctx, pubPEM, e.session.DeviceName, proton.CertificateFeatures{})
	}
	if err != nil {
		return err
	}
	e.session.Certificate = cert
	e.session.UpdatedAt = time.Now()
	e.markSessionDirty()
	e.logfOrNil("certificate: serial %s, expires %s", cert.SerialNumber, cert.ExpiresAt().Format(time.RFC3339))
	return nil
}

// sessionState is the non-secret summary returned by SessionState().
type sessionState struct {
	LoggedIn      bool   `json:"loggedIn"`
	UID           string `json:"uid,omitempty"`
	Email         string `json:"email,omitempty"`
	DeviceName    string `json:"deviceName,omitempty"`
	Plan          string `json:"plan,omitempty"`
	Tier          int    `json:"tier,omitempty"`
	CertSerial    string `json:"certSerial,omitempty"`
	CertExpiresAt string `json:"certExpiresAt,omitempty"`
	SessionDirty  bool   `json:"sessionDirty"`
	// AuthState is "unknown" until an authenticated call succeeds or fails,
	// then "valid" or "expired". The UI must not claim a session works just
	// because tokens are stored.
	AuthState      string `json:"authState"`
	LastVerifiedAt string `json:"lastVerifiedAt,omitempty"`
	AuthError      string `json:"authError,omitempty"`
}

func maskUID(uid string) string {
	if len(uid) <= 8 {
		return uid
	}
	return uid[:8] + "…"
}

// authStateName reports the last known authentication verdict.
func (e *engine) authStateName() string {
	switch e.authState {
	case "valid", "expired":
		return e.authState
	default:
		return "unknown"
	}
}

func (e *engine) sessionState() sessionState {
	st := sessionState{
		LoggedIn:     e.session.Access != "" || e.session.Refresh != "",
		UID:          maskUID(e.session.UID),
		Email:        redact(e.session.Email),
		DeviceName:   e.session.DeviceName,
		Plan:         e.plan,
		Tier:         e.tier,
		SessionDirty: e.sessionDirty,
		AuthState:    e.authStateName(),
		AuthError:    e.authErr,
	}
	if !e.authAt.IsZero() {
		st.LastVerifiedAt = e.authAt.UTC().Format(time.RFC3339)
	}
	if c := e.session.Certificate; c != nil {
		st.CertSerial = c.SerialNumber
		st.CertExpiresAt = c.ExpiresAt().Format(time.RFC3339)
	}
	return st
}

// marshalSession is what the app encrypts.
func (e *engine) marshalSession() (string, error) {
	buf, err := json.MarshalIndent(e.session, "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// isFingerprintConflict reports Proton's 409/2500 "client public key
// fingerprint conflict" answer, the one error that a key rotation fixes.
func isFingerprintConflict(err error) bool {
	apiErr, ok := err.(*proton.APIError)
	return ok && apiErr.Status == 409 && apiErr.Code == 2500
}
