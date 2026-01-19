package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
}

func NewService() *Service {
	return &Service{}
}

// GenerateToken generates a cryptographically secure random token.
//
// The token is generated using crypto/rand and encoded as hexadecimal.
//
// Parameters:
//   - length: Number of random bytes to generate (result will be 2*length hex characters)
//
// Returns:
//   - string: Hexadecimal encoded random token
//   - error: Error if random bytes cannot be generated
func (s *Service) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// HashToken securely hashes a token using bcrypt.
//
// Uses bcrypt's default cost (currently 10) which provides a good balance
// between security and performance.
//
// Parameters:
//   - token: The plain text token to hash
//
// Returns:
//   - string: bcrypt hash of the token
//   - error: Error if hashing fails
func (s *Service) HashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash token: %w", err)
	}
	return string(hash), nil
}

// VerifyToken verifies a token against its bcrypt hash.
//
// Parameters:
//   - token: The plain text token to verify
//   - hash: The bcrypt hash to verify against
//
// Returns:
//   - bool: True if the token matches the hash, false otherwise
func (s *Service) VerifyToken(token, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(token))
	return err == nil
}
