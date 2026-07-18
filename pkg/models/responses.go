package models

import (
	"strings"
	"time"
)

// ErrorOutput is the response envelope returned on error paths: the same shape
// as Output but with an empty Data and a populated Error list. Used in swaggo
// `@Failure` annotations so error responses have a concrete, documented schema.
type ErrorOutput struct {
	Data  []any   `json:"data"`
	Error []Error `json:"error"`
}

// MessageResponse documents handlers that reply with a bare {"message": "..."}
// object (e.g. delete confirmations) rather than the Output envelope.
type MessageResponse struct {
	Message string `json:"message"`
}

// TokenResponse documents the PASETO JSON token claims plus the encrypted token
// in the "data" field (see pasetoware.NewPayload). Retained for reference; the
// admin auth flow now returns TokenPairResponse.
type TokenResponse struct {
	Audience   string    `json:"aud"`
	Jti        string    `json:"jti"`
	Subject    string    `json:"sub"`
	IssuedAt   time.Time `json:"iat"`
	Expiration time.Time `json:"exp"`
	NotBefore  time.Time `json:"nbf"`
	Data       string    `json:"data"`
}

// Token types embedded in the PASETO "data" claim so a single symmetric key can
// mint both short-lived access tokens and long-lived refresh tokens while keeping
// them distinguishable (see EncodeTokenData / DecodeTokenData).
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// TokenPairResponse is the login / refresh response: a short-lived access token
// (sent on every request) and a long-lived refresh token (used only to mint a new
// pair). When the access token expires the client refreshes; when the refresh
// token ALSO expires, refresh is rejected and the user must log in again — i.e.
// they are fully logged out only once both tokens have expired.
type TokenPairResponse struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	ExpiresAt        time.Time `json:"expires_at"`         // access token expiry
	RefreshExpiresAt time.Time `json:"refresh_expires_at"` // refresh token expiry
	TokenType        string    `json:"token_type"`         // "Bearer"
	Data             string    `json:"data"`               // = AccessToken; backward-compat alias for older clients
}

// EncodeTokenData embeds the token type into the PASETO "data" claim.
// Format: "<type>:<subject>" (subject may itself contain ":").
func EncodeTokenData(tokenType, subject string) string {
	return tokenType + ":" + subject
}

// DecodeTokenData splits a value produced by EncodeTokenData back into its type
// and subject. ok is false if the value is not in the expected "<type>:<subject>"
// form (e.g. a legacy raw-subject token). Only the first ":" is treated as the
// separator, so subjects containing ":" round-trip correctly.
func DecodeTokenData(data string) (tokenType, subject string, ok bool) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
