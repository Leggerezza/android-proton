// SPDX-License-Identifier: GPL-3.0-or-later
package proton

import (
	"context"
	"fmt"
	"os"
	"time"
)

// CertificateRequest is the /vpn/v1/certificate payload.
type CertificateRequest struct {
	ClientPublicKey     string              `json:"ClientPublicKey"`
	ClientPublicKeyMode string              `json:"ClientPublicKeyMode"`
	Mode                string              `json:"Mode"`
	DeviceName          string              `json:"DeviceName"`
	Duration            string              `json:"Duration"`
	Features            CertificateFeatures `json:"Features"`
}

// CertificateFeatures are the per-device options Proton stores with the cert.
type CertificateFeatures struct {
	NetShieldLevel int  `json:"NetShieldLevel"`
	RandomNAT      bool `json:"RandomNAT"`
	SplitTCP       bool `json:"SplitTCP"`
	PortForwarding bool `json:"PortForwarding"`
}

// Certificate is the API answer.
type Certificate struct {
	Code                 int    `json:"Code"`
	SerialNumber         string `json:"SerialNumber"`
	ClientKeyFingerprint string `json:"ClientKeyFingerprint"`
	ClientKey            string `json:"ClientKey"`
	Certificate          string `json:"Certificate"`
	ExpirationTime       int64  `json:"ExpirationTime"`
	RefreshTime          int64  `json:"RefreshTime"`
	Mode                 string `json:"Mode"`
	DeviceName           string `json:"DeviceName"`
	ServerPublicKeyMode  string `json:"ServerPublicKeyMode"`
	ServerPublicKey      string `json:"ServerPublicKey"`
}

// ExpiresAt is when the certificate stops being valid.
func (c *Certificate) ExpiresAt() time.Time { return time.Unix(c.ExpirationTime, 0) }

// RefreshAt is when Proton wants the certificate renewed.
func (c *Certificate) RefreshAt() time.Time { return time.Unix(c.RefreshTime, 0) }

// Fresh reports whether the certificate is still usable for a while.
func (c *Certificate) Fresh(min time.Duration) bool {
	if c == nil || c.ExpirationTime == 0 {
		return false
	}
	return time.Until(c.ExpiresAt()) > min
}

// DefaultDeviceName returns a stable device name. Proton registers a device
// per (name, key) pair, so this must not change between runs.
func DefaultDeviceName() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "proton-stealth"
	}
	return "proton-stealth-" + host
}

// RequestCertificate registers our Ed25519 public key and returns the
// WireGuard device certificate.
func (c *Client) RequestCertificate(ctx context.Context, publicKeyPEM, deviceName string, features CertificateFeatures) (*Certificate, error) {
	if deviceName == "" {
		deviceName = DefaultDeviceName()
	}
	req := CertificateRequest{
		ClientPublicKey:     publicKeyPEM,
		ClientPublicKeyMode: "EC",
		Mode:                "persistent",
		DeviceName:          deviceName,
		Duration:            "10080 min", // 7 days
		Features:            features,
	}
	var cert Certificate
	if err := c.Post(ctx, "/vpn/v1/certificate", req, &cert); err != nil {
		return nil, fmt.Errorf("request certificate: %w", err)
	}
	if cert.Certificate == "" {
		return nil, fmt.Errorf("proton api returned an empty certificate")
	}
	return &cert, nil
}

// VPNAccount summarises plan information (used to cap the tier filters).
type VPNAccount struct {
	Code int `json:"Code"`
	VPN  struct {
		MaxTier    int    `json:"MaxTier"`
		PlanName   string `json:"PlanName"`
		PlanTitle  string `json:"PlanTitle"`
		MaxConnect int    `json:"MaxConnect"`
		Status     int    `json:"Status"`
	} `json:"VPN"`
}

// Account fetches the VPN plan.
func (c *Client) Account(ctx context.Context) (*VPNAccount, error) {
	var acc VPNAccount
	if err := c.Get(ctx, "/vpn/v2", &acc); err != nil {
		return nil, err
	}
	return &acc, nil
}
