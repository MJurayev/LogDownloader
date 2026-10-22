package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

const (
	SessionTTL       = 72 * time.Hour
	DownloadTokenTTL = 5 * time.Minute
)

var jwtSecret = []byte("logdownloader-secret-change-me")

func SetSecret(secret string) {
	if secret != "" {
		jwtSecret = []byte(secret)
	}
}

// LoadOrCreateSecret diskda saqlangan JWT secret'ni o'qiydi, mavjud bo'lmasa
// random 32-baytlik secret yaratadi va shu yo'lga 0600 huquq bilan yozadi.
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
		"typ":      "session",
		"user_id":  user.ID,
		"username": user.Username,
		"role":     string(user.Role),
		"exp":      time.Now().Add(SessionTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// GenerateDownloadToken — bitta job uchun qisqa muddatli (5 daqiqa) token.
// Brauzer <a download> Authorization header yubora olmagani uchun URL'da
// sessiya JWT o'rniga shu token yuriladi (reverse-proxy loglariga sessiya tushmaydi).
func GenerateDownloadToken(jobID, userID string) (string, error) {
	claims := jwt.MapClaims{
		"typ":     "download",
		"job_id":  jobID,
		"user_id": userID,
		"exp":     time.Now().Add(DownloadTokenTTL).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
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

var errBadToken = errors.New("invalid token")

// resolveToken — so'rovdan user ID ni aniqlaydi: Authorization: Bearer (sessiya)
// yoki `/download` yo'llarida `dt` query param (download token, job'ga bog'langan).
func resolveToken(r *http.Request) (string, error) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		claims, err := parseToken(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			return "", errBadToken
		}
		if typ, _ := claims["typ"].(string); typ != "" && typ != "session" {
			return "", errBadToken
		}
		uid, _ := claims["user_id"].(string)
		return uid, nil
	}
	if strings.HasSuffix(r.URL.Path, "/download") {
		if dt := r.URL.Query().Get("dt"); dt != "" {
			claims, err := parseToken(dt)
			if err != nil {
				return "", errBadToken
			}
			if typ, _ := claims["typ"].(string); typ != "download" {
				return "", errBadToken
			}
			// Token aynan shu job uchun bo'lishi shart: /api/jobs/{id}/download
			jobID, _ := claims["job_id"].(string)
			if jobID == "" || !strings.Contains(r.URL.Path, "/"+jobID+"/") {
				return "", errBadToken
			}
			uid, _ := claims["user_id"].(string)
			return uid, nil
		}
	}
	return "", nil
}

// Middleware `/api/*` (login'dan tashqari) uchun JWT tekshiradi. Boshqa barcha
// yo'llar (static fayllar, `/dl/{token}`, `/healthz`, `/metrics`) auth'siz o'tadi.
func Middleware(next http.Handler, lookup UserLookup) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		uid, err := resolveToken(r)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if uid == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		stored := lookup(uid)
		if stored == nil {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		user := &AuthUser{ID: stored.ID, Username: stored.Username, Role: stored.Role}
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

// ClientIP — X-Forwarded-For (birinchi), X-Real-IP yoki RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}
