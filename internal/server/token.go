package server

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInvalidToken indicates a token couldn't be validated.
	ErrInvalidToken = errors.New("invalid token")
)

const issuer = "library-portal"

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-me"
	}
	return []byte(s)
}

// GenerateToken creates a signed JWT containing the user ID.
func GenerateToken(userID int, expiry time.Duration) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		Subject:   strconv.Itoa(userID),
		ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		IssuedAt:  jwt.NewNumericDate(now),
		Issuer:    issuer,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(jwtSecret())
}

// ValidateToken parses and validates the token, returning the user ID.
func ValidateToken(tokenStr string) (int, error) {
	parser := jwt.NewParser(jwt.WithLeeway(5 * time.Second)) // clock skew tolerance

	token, err := parser.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{},
		func(t *jwt.Token) (interface{}, error) {

			// CRITICAL: enforce signing method
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return jwtSecret(), nil
		},
	)

	if err != nil {
		return 0, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return 0, ErrInvalidToken
	}

	// Validate issuer
	if claims.Issuer != issuer {
		return 0, ErrInvalidToken
	}

	if claims.Subject == "" {
		return 0, ErrInvalidToken
	}

	id, err := strconv.Atoi(claims.Subject)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}
