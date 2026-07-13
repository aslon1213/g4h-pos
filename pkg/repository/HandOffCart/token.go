package handoffcart_repo

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
)

const (
	// HandoffCodeDigits is the length of the typed handoff code. On its own it is
	// low-entropy; it is safe ONLY because it is single-use (consumed on claim),
	// short-lived (handoffTTL), state-gated (valid only in ready_for_handoff),
	// branch-scoped and rate-limited.
	HandoffCodeDigits = 8

	sessionTokenBytes = 32 // >= 128 bits of entropy for the entry-QR bearer
	handoffRefBytes   = 24 // higher-entropy QR reference carried alongside the code

	// handoffTTL is the short lifetime of a minted handoff token; maxHandoffAttempts
	// / handoffLockout bound repeated failed claims against a single live cart (the
	// claim endpoint is additionally rate-limited per client).
	handoffTTL         = 3 * time.Minute
	maxHandoffAttempts = 5
	handoffLockout     = 5 * time.Minute
)

// hashToken returns the hex sha-256 digest persisted in place of a plaintext
// token, so a leaked database row never reveals a usable credential.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// newSessionToken mints a high-entropy session token, returning the plaintext
// (shown once in the entry-QR link) and the digest to store.
func newSessionToken() (plaintext, hash string, err error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	return plaintext, hashToken(plaintext), nil
}

// newHandoffToken mints the checkout token in both forms: a higher-entropy QR
// reference and the typed HandoffCodeDigits-digit fallback code. Returns the
// plaintext of each plus their digests.
func newHandoffToken() (code, ref, codeHash, refHash string, err error) {
	code, err = randomDigits(HandoffCodeDigits)
	if err != nil {
		return "", "", "", "", err
	}
	rb := make([]byte, handoffRefBytes)
	if _, err := rand.Read(rb); err != nil {
		return "", "", "", "", err
	}
	ref = base64.RawURLEncoding.EncodeToString(rb)
	return code, ref, hashToken(code), hashToken(ref), nil
}

// randomDigits returns n cryptographically-random decimal digits, zero-padded.
func randomDigits(n int) (string, error) {
	upper := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
	v, err := rand.Int(rand.Reader, upper)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", n, v), nil
}

// endOfDay returns the last instant of t's calendar day in the POS timezone —
// the session token expiry ("live only 1 day based on created day").
func endOfDay(t time.Time) time.Time {
	loc := utils.GetTimeZone()
	t = t.In(loc)
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, 0, loc)
}
