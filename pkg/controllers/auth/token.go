package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/o1egl/paseto"
)

// defaultRefreshTokenExpiryHours is used when server.refresh_token_expiry_hours
// is not configured (30 days).
const defaultRefreshTokenExpiryHours = 24 * 30

// pasetoTokenSubject / pasetoTokenAudience mirror the (unexported) constants
// pasetoware embeds via CreateToken/NewPayload. We restate them here so a refresh
// token we decrypt manually is validated exactly like the middleware validates an
// access token. Keep in sync with github.com/gofiber/contrib/paseto.
const (
	pasetoTokenSubject  = "user-token"
	pasetoTokenAudience = "gofiber.gophers"
)

var (
	// ErrInvalidRefreshToken is returned when the presented token can't be
	// decrypted, is malformed, or is not a refresh token.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrExpiredRefreshToken is returned when the refresh token itself has
	// expired — the user must log in again (fully logged out).
	ErrExpiredRefreshToken = errors.New("refresh token has expired")
)

func (a *AuthControllers) accessTokenDuration() time.Duration {
	return time.Duration(a.TokenExpiryHours) * time.Minute
}

func (a *AuthControllers) refreshTokenDuration() time.Duration {
	hours := a.RefreshTokenExpiryHours
	if hours <= 0 {
		hours = defaultRefreshTokenExpiryHours
	}
	return time.Duration(hours) * time.Hour
}

// createTokenPair mints a fresh access + refresh token for the given subject
// (staff username). Both are PASETO local tokens encrypted with the same
// symmetric key; the token type is encoded in the "data" claim so they can be
// told apart on validation.
func (a *AuthControllers) createTokenPair(subject string) (*models.TokenPairResponse, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(a.SecretSymmetricKey)
	if err != nil {
		return nil, err
	}

	accessDur := a.accessTokenDuration()
	refreshDur := a.refreshTokenDuration()

	accessToken, err := pasetoware.CreateToken(
		keyBytes, models.EncodeTokenData(models.TokenTypeAccess, subject), accessDur, pasetoware.PurposeLocal,
	)
	if err != nil {
		return nil, err
	}

	refreshToken, err := pasetoware.CreateToken(
		keyBytes, models.EncodeTokenData(models.TokenTypeRefresh, subject), refreshDur, pasetoware.PurposeLocal,
	)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &models.TokenPairResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresAt:        now.Add(accessDur),
		RefreshExpiresAt: now.Add(refreshDur),
		TokenType:        "Bearer",
		Data:             accessToken,
	}, nil
}

// parseRefreshToken decrypts and validates a refresh token, returning its subject
// (staff username). It mirrors pasetoware's default validation (expiry, subject,
// audience) and additionally requires the token to be of the refresh type — an
// access token presented here is rejected. An expired refresh token yields
// ErrExpiredRefreshToken (the "fully logged out" case).
func (a *AuthControllers) parseRefreshToken(token string) (string, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(a.SecretSymmetricKey)
	if err != nil {
		return "", err
	}

	var raw []byte
	if err := paseto.NewV2().Decrypt(token, keyBytes, &raw, nil); err != nil {
		return "", ErrInvalidRefreshToken
	}

	var payload paseto.JSONToken
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ErrInvalidRefreshToken
	}

	if time.Now().After(payload.Expiration) {
		return "", ErrExpiredRefreshToken
	}
	if err := payload.Validate(
		paseto.ValidAt(time.Now()),
		paseto.Subject(pasetoTokenSubject),
		paseto.ForAudience(pasetoTokenAudience),
	); err != nil {
		return "", ErrInvalidRefreshToken
	}

	tokenType, subject, ok := models.DecodeTokenData(payload.Get("data"))
	if !ok || tokenType != models.TokenTypeRefresh {
		return "", ErrInvalidRefreshToken
	}
	return subject, nil
}
