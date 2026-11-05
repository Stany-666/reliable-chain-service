package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateToken(userID int64, secret []byte) (string, error) {
	// 1. 准备 token 携带的信息
	if len(secret) < 32 {
		return "", fmt.Errorf("JWT signing secret must be at least 32 bytes")
	}
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	// 2. 选择签名算法，用 claims 创建 token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 3. 用 secret 签名，返回 token 字符串和错误
	return token.SignedString(secret)
}

func VerifyToken(tokenString string, secret []byte) (int64, error) {
	if len(secret) < 32 {
		return 0, fmt.Errorf("JWT signing secret must be at least 32 bytes")
	}

	var claims jwt.RegisteredClaims

	_, err := jwt.ParseWithClaims(
		tokenString,
		&claims,
		func(token *jwt.Token) (any, error) {
			return secret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return 0, fmt.Errorf("verify JWT: %w", err)
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("invalid JWT user ID")
	}

	return userID, nil

}
