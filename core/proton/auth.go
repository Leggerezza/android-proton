// SPDX-License-Identifier: GPL-3.0-or-later
package proton

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	srp "github.com/ProtonMail/go-srp"
)

type authInfoRequest struct {
	Username string `json:"Username"`
	Intent   string `json:"Intent"`
}

type authInfoResponse struct {
	Code            int    `json:"Code"`
	Version         int    `json:"Version"`
	Modulus         string `json:"Modulus"`
	ServerEphemeral string `json:"ServerEphemeral"`
	Salt            string `json:"Salt"`
	SRPSession      string `json:"SRPSession"`
	TwoFA           struct {
		Enabled int `json:"Enabled"`
		TOTP    int `json:"TOTP"`
	} `json:"2FA"`
}

type authRequest struct {
	Username        string `json:"Username"`
	ClientEphemeral string `json:"ClientEphemeral"`
	ClientProof     string `json:"ClientProof"`
	SRPSession      string `json:"SRPSession"`
}

type authResponse struct {
	Code         int      `json:"Code"`
	AccessToken  string   `json:"AccessToken"`
	RefreshToken string   `json:"RefreshToken"`
	TokenType    string   `json:"TokenType"`
	UID          string   `json:"UID"`
	ServerProof  string   `json:"ServerProof"`
	Scope        string   `json:"Scope"`
	Scopes       []string `json:"Scopes"`
	TwoFA        struct {
		Enabled int `json:"Enabled"`
		TOTP    int `json:"TOTP"`
	} `json:"2FA"`
}

// Login performs Proton's SRP login and stores the session on the client.
// The password never leaves this process in clear text: only the SRP proof
// is sent.
func (c *Client) Login(ctx context.Context, username, password string) (*AuthResult, error) {
	var info authInfoResponse
	if err := c.CorePost(ctx, "/auth/info", authInfoRequest{Username: username, Intent: "Proton"}, &info, false); err != nil {
		return nil, fmt.Errorf("auth info: %w", err)
	}
	if info.Modulus == "" || info.Salt == "" {
		return nil, fmt.Errorf("auth info: incomplete SRP parameters (account may require a different login flow)")
	}

	auth, err := srp.NewAuth(info.Version, username, []byte(password), info.Salt, info.Modulus, info.ServerEphemeral)
	if err != nil {
		return nil, fmt.Errorf("srp init: %w", err)
	}
	proofs, err := auth.GenerateProofs(2048)
	if err != nil {
		return nil, fmt.Errorf("srp proofs: %w", err)
	}

	req := authRequest{
		Username:        username,
		ClientEphemeral: base64.StdEncoding.EncodeToString(proofs.ClientEphemeral),
		ClientProof:     base64.StdEncoding.EncodeToString(proofs.ClientProof),
		SRPSession:      info.SRPSession,
	}
	var resp authResponse
	if err := c.CorePost(ctx, "/auth/v4", req, &resp, false); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	if resp.ServerProof != "" {
		expected := base64.StdEncoding.EncodeToString(proofs.ExpectedServerProof)
		if resp.ServerProof != expected {
			return nil, fmt.Errorf("server proof mismatch: the API did not prove knowledge of the password")
		}
	}

	c.setTokens(Tokens{UID: resp.UID, Access: resp.AccessToken, Refresh: resp.RefreshToken})
	return &AuthResult{
		UID:      resp.UID,
		Needs2FA: needs2FA(resp),
		Scopes:   resp.Scopes,
	}, nil
}

func needs2FA(resp authResponse) bool {
	if resp.TwoFA.Enabled == 0 {
		return false
	}
	for _, s := range resp.Scopes {
		if s == "vpn" {
			return false
		}
	}
	return !strings.Contains(resp.Scope, "vpn")
}

// AuthResult is what Login returns.
type AuthResult struct {
	UID      string
	Needs2FA bool
	Scopes   []string
}

// Submit2FA completes a login that requires a TOTP code.
func (c *Client) Submit2FA(ctx context.Context, code string) error {
	var out struct {
		Code   int      `json:"Code"`
		Scope  string   `json:"Scope"`
		Scopes []string `json:"Scopes"`
	}
	if err := c.CorePost(ctx, "/auth/2fa", map[string]string{"TwoFactorCode": code}, &out, true); err != nil {
		return fmt.Errorf("2fa: %w", err)
	}
	return nil
}

// Logout invalidates the current session.
func (c *Client) Logout(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.CoreBase+"/auth/v4", nil)
	if err != nil {
		return err
	}
	t := c.Tokens()
	req.Header.Set("x-pm-appversion", c.AppVersion)
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Authorization", "Bearer "+t.Access)
	if t.UID != "" {
		req.Header.Set("x-pm-uid", t.UID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &APIError{Status: resp.StatusCode}
	}
	return nil
}
