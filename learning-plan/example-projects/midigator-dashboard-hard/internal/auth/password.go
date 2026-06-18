package auth

import "golang.org/x/crypto/bcrypt"

// Hash returns a bcrypt hash of a secret (password or PIN).
func Hash(secret string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	return string(b), err
}

// Check reports whether secret matches the bcrypt hash.
func Check(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}
