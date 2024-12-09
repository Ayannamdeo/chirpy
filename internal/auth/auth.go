package auth

import (
	"time"
	"net/http"
  "errors"
  "strings"
  "log"
  "crypto/rand"
  "encoding/hex"
	"github.com/google/uuid"
  "golang.org/x/crypto/bcrypt"
  "github.com/golang-jwt/jwt/v5"
)

func HashPassword (password string) (string, error){
  hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
  if err != nil {
    return "", err
  }
  return string(hashedBytes), nil
}

func CheckPasswordHash (password, hash string) error {
  return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error){
  // log.Println("")
  // log.Println("current time:")
  // log.Println(jwt.NewNumericDate(time.Now()))
  // log.Printf("Setting expiration time to: %v", time.Now().Add(expiresIn))
  // log.Println("expiresIn :")
  // log.Println(jwt.NewNumericDate(time.Now().Add(expiresIn)))
  // log.Println("")
  token := jwt.NewWithClaims(jwt.SigningMethodHS256,
    jwt.RegisteredClaims{Issuer: "chirpy", Subject: userID.String(), IssuedAt:
      jwt.NewNumericDate(time.Now()), ExpiresAt:
      jwt.NewNumericDate(time.Now().Add(expiresIn))})
  signedToken, err := token.SignedString([]byte(tokenSecret))
  if err != nil {
    return "", err
  }
  return signedToken, nil
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error){
  keyFunc := func(token *jwt.Token) (interface{}, error) {
    return []byte(tokenSecret), nil
  }
  claims := jwt.RegisteredClaims{}
  token, err := jwt.ParseWithClaims(tokenString, &claims, keyFunc)
  if err != nil || !token.Valid {
    log.Printf("parsewithclaims failed %v\n", err)
    return uuid.UUID{}, err
  }
  userId, err := uuid.Parse(claims.Subject)
  if err != nil {
    return uuid.UUID{}, err
  }
  return userId, nil
}

func GetBearerToken(headers http.Header) (string, error){
  authHeader := headers.Get("Authorization")
  if authHeader == "" {
    return "", errors.New("header Authorization doesn't exist")
  }
  tokenSlice := strings.Split(authHeader, " ")
  // Check if the Bearer token format is correct
  if len(tokenSlice) != 2 || tokenSlice[0] != "Bearer" {
    return "", errors.New("invalid Bearer token format")
  }
  tokenString := tokenSlice[1]
  return tokenString, nil
}

func MakeRefreshToken() (string, error){
  randData := make([]byte, 32)
  _, err := rand.Read(randData)
  if err != nil {
    return "", err
  }
  encodedStr := hex.EncodeToString(randData)
  return encodedStr, nil
}
