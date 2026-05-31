package models

import "time"

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

// TokenResponse documents the login response: the PASETO JSON token claims plus
// the encrypted token in the "data" field (see pasetoware.NewPayload).
type TokenResponse struct {
	Audience   string    `json:"aud"`
	Jti        string    `json:"jti"`
	Subject    string    `json:"sub"`
	IssuedAt   time.Time `json:"iat"`
	Expiration time.Time `json:"exp"`
	NotBefore  time.Time `json:"nbf"`
	Data       string    `json:"data"`
}
