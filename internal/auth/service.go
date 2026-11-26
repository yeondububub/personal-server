package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"personal-server/internal/config"
	"personal-server/internal/httputil"
)

const sessionCookieName = "ps_session"

type contextKey string

const userContextKey contextKey = "auth_user"

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Service struct {
	db           *sql.DB
	cookieSecure bool
	sessionTTL   time.Duration
	limiter      *LoginLimiter
}

func NewService(conn *sql.DB, cfg config.Config) (*Service, error) {
	svc := &Service{
		db:           conn,
		cookieSecure: cfg.Server.CookieSecure,
		sessionTTL:   time.Duration(cfg.Auth.SessionTTLHours) * time.Hour,
		limiter:      NewLoginLimiter(5, 5*time.Minute),
	}
	if err := svc.ensureInitialAdmin(cfg.Auth.InitialAdminUsername, cfg.Auth.InitialAdminPassword); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) ensureInitialAdmin(username, password string) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("initial admin username and password must not be empty")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username,
		hash,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert initial admin: %w", err)
	}
	return nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userContextKey).(User)
	return u, ok
}

func (s *Service) AuthenticateRequest(r *http.Request) (User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return User{}, errors.New("missing session")
	}
	now := time.Now().Unix()
	var u User
	err = s.db.QueryRow(
		`SELECT u.id, u.username
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at > ?`,
		hashToken(cookie.Value),
		now,
	).Scan(&u.ID, &u.Username)
	if err != nil {
		return User{}, errors.New("invalid or expired session")
	}
	return u, nil
}

func (s *Service) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.AuthenticateRequest(r)
		if err != nil {
			httputil.WriteError(w, http.StatusUnauthorized, "로그인이 필요합니다.")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Service) HandleLogin(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := s.limiter.Allow(r)
		if !allowed {
			secs := int(math.Ceil(retryAfter.Seconds()))
			httputil.WriteError(w, http.StatusTooManyRequests, fmt.Sprintf("로그인 시도가 너무 많습니다. %d초 후 다시 시도하세요.", secs))
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
			return
		}

		var user User
		var pwHash string
		err := s.db.QueryRow(
			`SELECT id, username, password_hash FROM users WHERE username = ?`,
			strings.TrimSpace(req.Username),
		).Scan(&user.ID, &user.Username, &pwHash)
		if err != nil {
			s.limiter.RecordFailure(r)
			httputil.WriteError(w, http.StatusUnauthorized, "아이디 또는 비밀번호가 올바르지 않습니다.")
			return
		}

		ok, err := VerifyPassword(req.Password, pwHash)
		if err != nil || !ok {
			s.limiter.RecordFailure(r)
			httputil.WriteError(w, http.StatusUnauthorized, "아이디 또는 비밀번호가 올바르지 않습니다.")
			return
		}

		s.limiter.Reset(r)
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())

		rawToken, err := generateSessionToken()
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "세션 생성에 실패했습니다.")
			return
		}
		now := time.Now()
		expiresAt := now.Add(s.sessionTTL)
		if _, err := s.db.Exec(
			`INSERT INTO sessions (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
			hashToken(rawToken),
			user.ID,
			expiresAt.Unix(),
			now.Unix(),
		); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "세션 저장에 실패했습니다.")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    rawToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.cookieSecure,
			SameSite: http.SameSiteStrictMode,
			Expires:  expiresAt,
		})

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"accessTier":    accessTier,
			"user":          user,
		})
	}
}

func (s *Service) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Service) HandleMe(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.AuthenticateRequest(r)
		if err != nil {
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"authenticated": false,
				"accessTier":    accessTier,
				"user":          nil,
			})
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"accessTier":    accessTier,
			"user":          u,
		})
	}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Service) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
		return
	}
	if len(req.NewPassword) < 8 {
		httputil.WriteError(w, http.StatusBadRequest, "새 비밀번호는 8자 이상이어야 합니다.")
		return
	}

	var currentHash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&currentHash); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "계정 정보를 읽을 수 없습니다.")
		return
	}
	valid, err := VerifyPassword(req.CurrentPassword, currentHash)
	if err != nil || !valid {
		httputil.WriteError(w, http.StatusBadRequest, "현재 비밀번호가 일치하지 않습니다.")
		return
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "비밀번호 해싱에 실패했습니다.")
		return
	}
	if _, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, newHash, u.ID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "비밀번호 변경에 실패했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
