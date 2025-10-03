// go
package auth

import (
    "testing"
    "time"
    "github.com/google/uuid"
    "github.com/golang-jwt/jwt/v5"
    "fmt"
    "net/http"
)

func TestMakeJWT_Claims(t *testing.T) {
    userID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
    if err != nil {
        t.Fatalf("parse uuid: %v", err)
    }
    secret := "test-secret"
    expiresIn := 1 * time.Minute
    tok, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    keyfunc := func(token *jwt.Token) (interface{}, error) {
        if token.Method != jwt.SigningMethodHS256 {
            return nil, fmt.Errorf("unexpected alg: %s", token.Method.Alg())
        }
        return []byte(secret), nil
    }

    parsed, err := jwt.ParseWithClaims(tok, &jwt.RegisteredClaims{}, keyfunc)
    if err != nil {
        t.Fatalf("parse failed: %v", err)
    }

    claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
    if !ok || !parsed.Valid {
        t.Fatalf("invalid claims")
    }

    if claims.Issuer != "chirpy" {
        t.Fatalf("issuer = %q", claims.Issuer)
    }

    if claims.Subject != userID.String() {
        t.Fatalf("subject = %q", claims.Subject)
    }

    if claims.IssuedAt == nil || claims.ExpiresAt == nil {
        t.Fatalf("timestamps not set")
    }

    if !claims.ExpiresAt.Time.After(claims.IssuedAt.Time) {
        t.Fatalf("exp not after iat")
    }
}

func TestValidateJWT_Valid(t *testing.T) {
    userID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
    if err != nil {
        t.Fatalf("parse uuid: %v", err)
    }
    secret := "test-secret"
    expiresIn := 1 * time.Minute

    tok, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    uID, err := ValidateJWT(tok, secret)
    if err != nil {
        t.Fatalf("ValidateJWT failure: %v",err)
    }

    if uID != userID {
        t.Fatalf("%v not equal to %v", uID, userID)
    }
}

func TestValidateJWT_Expired(t *testing.T) {
    userID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
    if err != nil {
        t.Fatalf("parse uuid: %v", err)
    }
    secret := "test-secret"
    expiresIn := 1 * time.Second

    tok, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    time.Sleep(5 * time.Second)

    _, err = ValidateJWT(tok, secret)
    if err == nil {
        t.Fatalf("expected ValidateJWT to return an error for expired token, but it didn't")
    }
}

func TestValidateJWT_WrongSecret(t *testing.T) {
    userID, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
    if err != nil {
        t.Fatalf("parse uuid: %v", err)
    }
    secret_A := "test-secret"
    secret_B := "secret-test"
    expiresIn := 5 * time.Second

    tok, err := MakeJWT(userID, secret_A, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    _, err = ValidateJWT(tok, secret_B)
    if err == nil {
        t.Fatalf("expected ValidateJWT to return an error for wrong secret")
    }
}

func TestValidateGBT_CorrectToken(t *testing.T) {
    headers := http.Header{}
    headers.Set("Authorization", "Bearer fart")
    token, err := GetBearerToken(headers)

    if err != nil {
        t.Fatalf("expected nil error, got %v", err)
    }
    if token != "fart" {
        t.Fatalf("expected token %q, got %q", "fart", token)
    }
} 

func TestValidateGBT_MissingHeader(t *testing.T) {
    headers := http.Header{}
    token, err := GetBearerToken(headers)
    if err == nil {
        t.Fatalf("expected missing header error")
    }
    if token != "" {
        t.Fatalf("expected empty token, got %q", token)
    }
}

func TestValidateGBT_MissingHeaderPart(t *testing.T) {
    headers := http.Header{}
    headers.Set("Authorization", "fart")
    _, err := GetBearerToken(headers)
    if err == nil {
        t.Fatalf("expected Authorization header")
    }
}