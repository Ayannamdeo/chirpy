package auth

import (
	"log"
  "golang.org/x/crypto/bcrypt"
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