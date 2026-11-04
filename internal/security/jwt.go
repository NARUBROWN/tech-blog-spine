package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

var ErrInvalidToken = errors.New("유효하지 않은 토큰")

type TokenKind string

const (
	AccessToken  TokenKind = "access"
	RefreshToken TokenKind = "refresh"
)

type Claims struct {
	ID        string    `json:"jti,omitempty"`
	Subject   string    `json:"sub"`
	UserID    int64     `json:"userId"`
	TokenKind TokenKind `json:"tokenType"`
	IssuedAt  int64     `json:"iat"`
	ExpiresAt int64     `json:"exp"`
}

type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	secure     bool
	now        func() time.Time
}

func NewTokenManager(config config.Config) *TokenManager {
	return &TokenManager{
		secret: []byte(config.JWTSecret), accessTTL: config.AccessTokenTTL,
		refreshTTL: config.RefreshTokenTTL, secure: config.CookieSecure, now: time.Now,
	}
}

func (m *TokenManager) CreateAccessToken(userID int64, username string) (string, error) {
	return m.create(userID, username, AccessToken, m.accessTTL)
}

func (m *TokenManager) CreateRefreshToken(userID int64, username string) (string, error) {
	return m.create(userID, username, RefreshToken, m.refreshTTL)
}

func (m *TokenManager) Parse(token string, expected TokenKind) (Claims, error) {
	return m.parse(token, expected, true)
}

func (m *TokenManager) ParseSigned(token string, expected TokenKind) (Claims, error) {
	return m.parse(token, expected, false)
}

func (m *TokenManager) parse(token string, expected TokenKind, checkExpiry bool) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	signed := parts[0] + "." + parts[1]
	want := m.sign(signed)
	got, err := decodeSegment(parts[2])
	if err != nil || len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
		return Claims{}, ErrInvalidToken
	}
	headerBytes, err := decodeSegment(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return Claims{}, ErrInvalidToken
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if json.Unmarshal(payload, &claims) != nil || claims.UserID <= 0 || claims.Subject == "" || claims.TokenKind != expected {
		return Claims{}, ErrInvalidToken
	}
	if claims.IssuedAt > m.now().Add(time.Minute).Unix() || (checkExpiry && claims.ExpiresAt <= m.now().Unix()) {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func decodeSegment(encoded string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, ErrInvalidToken
	}
	return decoded, nil
}

func (m *TokenManager) create(userID int64, username string, kind TokenKind, ttl time.Duration) (string, error) {
	if userID <= 0 || strings.TrimSpace(username) == "" {
		return "", fmt.Errorf("토큰 주체가 올바르지 않습니다")
	}
	now := m.now()
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims := Claims{
		Subject: username, UserID: userID, TokenKind: kind,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
	}
	if kind == RefreshToken {
		// 같은 초에 로그인해도 브라우저마다 독립된 리프레시 토큰을 발급합니다.
		var nonce [32]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return "", err
		}
		claims.ID = base64.RawURLEncoding.EncodeToString(nonce[:])
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(m.sign(unsigned)), nil
}

func (m *TokenManager) sign(value string) []byte {
	digest := hmac.New(sha256.New, m.secret)
	_, _ = digest.Write([]byte(value))
	return digest.Sum(nil)
}

func (m *TokenManager) AccessCookie(token string) httpx.Cookie {
	return httpx.Cookie{
		Name: httpx.AccessTokenCookieName, Value: token, Path: "/", MaxAge: int(m.accessTTL.Seconds()),
		HttpOnly: true, Secure: m.secure, SameSite: m.sameSite(),
	}
}

func (m *TokenManager) RefreshCookie(token string) httpx.Cookie {
	return httpx.Cookie{
		Name: httpx.RefreshTokenCookieName, Value: token, Path: "/", MaxAge: int(m.refreshTTL.Seconds()),
		HttpOnly: true, Secure: m.secure, SameSite: m.sameSite(),
	}
}

func (m *TokenManager) ClearCookies() []httpx.Cookie {
	expires := time.Unix(1, 0).UTC()
	return []httpx.Cookie{
		{Name: httpx.AccessTokenCookieName, Path: "/", MaxAge: -1, Expires: &expires, HttpOnly: true, Secure: m.secure, SameSite: m.sameSite()},
		{Name: httpx.RefreshTokenCookieName, Path: "/", MaxAge: -1, Expires: &expires, HttpOnly: true, Secure: m.secure, SameSite: m.sameSite()},
	}
}

func (m *TokenManager) sameSite() httpx.SameSite {
	if m.secure {
		return httpx.SameSiteNone
	}
	return httpx.SameSiteLax
}

func (m *TokenManager) AccessTTL() time.Duration  { return m.accessTTL }
func (m *TokenManager) RefreshTTL() time.Duration { return m.refreshTTL }
