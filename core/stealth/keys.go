package stealth

import (
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// KeyPair mirrors Proton's go-vpn-lib/ed25519 key handling: the API is given
// an Ed25519 public key (PKIX PEM) and the WireGuard private key is the
// X25519 secret derived from the same Ed25519 seed. That is how the WireGuard
// peer is authorised: the certificate registers the Ed25519 key, and the
// server accepts the derived X25519 key.
type KeyPair struct {
	Private ed25519.PrivateKey
}

// NewKeyPair generates a fresh key pair.
func NewKeyPair() (*KeyPair, error) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	return &KeyPair{Private: priv}, nil
}

// KeyPairFromSeed restores a key pair from its 32-byte seed.
func KeyPairFromSeed(seed []byte) (*KeyPair, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("ed25519 seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	return &KeyPair{Private: ed25519.NewKeyFromSeed(seed)}, nil
}

// Seed returns the 32-byte Ed25519 seed (the part worth persisting).
func (k *KeyPair) Seed() []byte { return k.Private.Seed() }

// PublicKeyPEM returns the PKIX PEM encoding sent as ClientPublicKey.
func (k *KeyPair) PublicKeyPEM() (string, error) {
	der, err := x509.MarshalPKIXPublicKey(k.Private.Public())
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// X25519PrivateKey derives the WireGuard private key from the Ed25519 key
// exactly like Proton does (SHA-512 of the seed, clamped).
func (k *KeyPair) X25519PrivateKey() []byte {
	h := sha512.Sum512(k.Private.Seed())
	h[0] &= 0xF8
	h[31] &= 0x7F
	h[31] |= 0x40
	out := make([]byte, 32)
	copy(out, h[:32])
	return out
}

// X25519PublicKey returns the derived WireGuard public key.
func (k *KeyPair) X25519PublicKey() ([]byte, error) {
	return curve25519.X25519(k.X25519PrivateKey(), curve25519.Basepoint)
}

// X25519PrivateKeyBase64 is the wg-quick style representation.
func (k *KeyPair) X25519PrivateKeyBase64() string {
	return base64.StdEncoding.EncodeToString(k.X25519PrivateKey())
}
