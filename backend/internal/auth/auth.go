package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"logdownloader/internal/model"
)

type ctxKey string

const userCtxKey ctxKey = "user"

var jwtSecret = []byte("logdownloader-secret-change-me")

func SetSecret(secret string) {
	if secret != "" {
		jwtSecret = []byte(secret)
	}
}

// LoadOrCreateSecret diskda saqlangan JWT secret'ni o'qiydi, mavjud bo'lmasa
// random 32-baytlik secret yaratadi va shu yo'lga 0600 huquq bilan yozadi.
// Shu sababli server qayta ishga tushganda chiqarilgan tokenlar yaroqli qoladi.
func LoadOrCreateSecret(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			return s, nil
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	secret := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		return "", fmt.Errorf("write secret: %w", err)
	}
	return secret, nil
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func GenerateToken(user model.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     string(user.Role),
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func parseToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}

type AuthUser struct {
	ID       string
	Username string
	Role     model.Role
}

func GetUser(r *http.Request) *AuthUser {
	u, _ := r.Context().Value(userCtxKey).(*AuthUser)
	return u
}

// UserLookup token'dagi user ID bo'yicha joriy userni store'dan qaytaradi.
// nil bo'lsa (user o'chirilgan) token rad etiladi. Username va rol ham
// store'dan olinadi — rol o'zgarishi re-login'siz, keyingi so'rovdan kuchga kiradi.
type UserLookup func(id string) *model.User

// Middleware `/api/*` (login'dan tashqari) uchun JWT tekshiradi. Boshqa barcha
// yo'llar (static fayllar, `/dl/{token}`) auth'siz o'tadi.
func Middleware(next http.Handler, lookup UserLookup) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		tokenStr := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}

		// Brauzer <a download> Authorization header yubora olmaydi. Native
		// streaming download uchun /api/.../download endpoint'iga ?token=
		// query param orqali ham auth qabul qilamiz (faqat shu suffix uchun).
		if tokenStr == "" && strings.HasSuffix(r.URL.Path, "/download") {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		claims, err := parseToken(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		uid, _ := claims["user_id"].(string)
		if uid == "" {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		stored := lookup(uid)
		if stored == nil {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		user := &AuthUser{
			ID:       stored.ID,
			Username: stored.Username,
			Role:     stored.Role,
		}

		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil || user.Role != model.RoleAdmin {
			http.Error(w, "admin access required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
