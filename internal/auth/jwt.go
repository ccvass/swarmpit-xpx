package auth

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/ccvass/swarmpit-xpx/internal/store"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Usr struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"usr"`
	jwt.RegisteredClaims
}

func GenerateJWT(username, role string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "swarmpit",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	claims.Usr.Username = username
	claims.Usr.Role = role
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(store.GetSecret()))
}

func ValidateJWT(tokenStr string) (*Claims, error) {
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return []byte(store.GetSecret()), nil
	})
	return claims, err
}

func DecodeBasic(header string) (string, string, bool) {
	header = strings.TrimPrefix(header, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Middleware

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			w.Header().Set("X-Backend-Server", "swarmpit")
			http.Error(w, `{"error":"Authentication failed"}`, http.StatusUnauthorized)
			return
		}
		claims, err := ValidateJWT(token)
		if err != nil {
			w.Header().Set("X-Backend-Server", "swarmpit")
			http.Error(w, `{"error":"Token invalid"}`, http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-User", claims.Usr.Username)
		r.Header.Set("X-Role", claims.Usr.Role)
		next.ServeHTTP(w, r)
	})
}

func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Role") != "admin" {
			http.Error(w, `{"error":"Admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
