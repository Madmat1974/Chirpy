package auth

import (
	"log"
	"github.com/alexedwards/argon2id"
	"time"
	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"net/http"
	"fmt"
	"crypto/rand"
	"encoding/hex"
)

func HashPassword(password string) (string, error){
	h, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err!= nil {
		log.Fatal(err)
	}
	return h, err
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		log.Fatal(err)
	}
	return match, err
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:		"chirpy",
		IssuedAt:	jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt:	jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:	userID.String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(tokenSecret))
	if err != nil {
		return "", err
	}
	return ss, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	claimsStruct := jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		&claimsStruct,
		func(token *jwt.Token) (interface{}, error) {
			return []byte(tokenSecret), nil
		},
	)
	if err != nil {
		return uuid.Nil, err
	}
	subjectString, err := token.Claims.GetSubject()
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(subjectString)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func GetBearerToken(headers http.Header) (string, error) {
    auth := headers.Get("Authorization")
    if auth == "" {
        return "", fmt.Errorf("missing Authorization header")
    }
    if !strings.HasPrefix(auth, "Bearer ") {
        return "", fmt.Errorf("invalid Authorization header")
    }
    token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
    if token == "" {
        return "", fmt.Errorf("empty bearer token")
    }
    return token, nil
}

func MakeRefreshToken() (string, error) {
	key := make([]byte, 32)
	rand.Read(key)
	encodedKey := hex.EncodeToString(key)
	return encodedKey, nil
}

func GetAPIKey(headers http.Header) (string, error) {
	auth := headers.Get("Authorization")
	if auth == "" {
		return "", fmt.Errorf("missing Authorization header")
	}
	if !strings.HasPrefix(auth, "ApiKey ") {
		return "", fmt.Errorf("invalid Authorization header")
	}
	key := strings.TrimSpace(strings.TrimPrefix(auth, "ApiKey "))
	if key == "" {
		fmt.Errorf("empty apikey")
	}
	return key, nil
}