// Package auth issues and verifies JWT access tokens and hashes credentials.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned for any malformed/expired/invalid token.
var ErrInvalidToken = errors.New("invalid token")

// Claims is the JWT payload. Imp (impersonator) is set while a platform admin
// is acting as a tenant user, so "stop impersonating" can restore them.
type Claims struct {
	UserID int64  `json:"uid"`
	Imp    *int64 `json:"imp,omitempty"`
	jwt.RegisteredClaims
}

// TokenManager signs and parses access tokens with a shared HMAC secret.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenManager builds a manager from the configured secret and token TTL.
func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl}
}

// Issue mints a signed token for a user. imp is the impersonator id, or nil.
func (m *TokenManager) Issue(userID int64, imp *int64, now time.Time) (string, time.Time, error) {
	exp := now.Add(m.ttl)
	claims := Claims{
		UserID: userID,
		Imp:    imp,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// Parse verifies a token and returns its claims.
func (m *TokenManager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
