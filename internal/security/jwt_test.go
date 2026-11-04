package security

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

func TestTokenManagerSeparatesAccessAndRefreshTokens(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager(config.Config{
		JWTSecret:       "a-test-secret-that-is-longer-than-32-bytes",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	manager.now = func() time.Time { return now }

	accessToken, err := manager.CreateAccessToken(7, "tester")
	if err != nil {
		t.Fatalf("액세스 토큰 생성 실패: %v", err)
	}
	claims, err := manager.Parse(accessToken, AccessToken)
	if err != nil {
		t.Fatalf("액세스 토큰 검증 실패: %v", err)
	}
	if claims.UserID != 7 || claims.Subject != "tester" {
		t.Fatalf("토큰 주체가 다릅니다: %+v", claims)
	}
	if _, err := manager.Parse(accessToken, RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("액세스 토큰을 리프레시 토큰으로 허용했습니다: %v", err)
	}

	manager.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := manager.Parse(accessToken, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("만료된 액세스 토큰을 허용했습니다: %v", err)
	}
	if _, err := manager.ParseSigned(accessToken, AccessToken); err != nil {
		t.Fatalf("로그아웃용 서명 검증은 만료 토큰도 식별해야 합니다: %v", err)
	}
}

func TestRefreshTokensDifferForLoginsInTheSameSecond(t *testing.T) {
	manager := NewTokenManager(config.Config{
		JWTSecret:      "a-test-secret-that-is-longer-than-32-bytes",
		AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour,
	})
	manager.now = func() time.Time { return time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC) }
	first, err := manager.CreateRefreshToken(7, "tester")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreateRefreshToken(7, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("동시 로그인에 동일한 리프레시 토큰이 발급됐습니다")
	}
	for _, token := range []string{first, second} {
		claims, err := manager.Parse(token, RefreshToken)
		if err != nil || claims.ID == "" || claims.UserID != 7 {
			t.Fatalf("독립 세션 토큰이 올바르지 않습니다: %+v, %v", claims, err)
		}
	}
}

func TestTokenManagerAcceptsLegacyRefreshTokenWithoutID(t *testing.T) {
	manager := NewTokenManager(config.Config{JWTSecret: "a-test-secret-that-is-longer-than-32-bytes"})
	now := time.Now()
	payload, err := json.Marshal(Claims{Subject: "tester", UserID: 7, TokenKind: RefreshToken, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(manager.sign(unsigned))
	if claims, err := manager.Parse(token, RefreshToken); err != nil || claims.ID != "" || claims.UserID != 7 {
		t.Fatalf("기존 로그인 토큰과의 호환성이 깨졌습니다: %+v, %v", claims, err)
	}
}

func TestTokenManagerRejectsTampering(t *testing.T) {
	manager := NewTokenManager(config.Config{
		JWTSecret:       "a-test-secret-that-is-longer-than-32-bytes",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	token, err := manager.CreateAccessToken(1, "tester")
	if err != nil {
		t.Fatal(err)
	}
	tampered := token[:len(token)-1] + "A"
	if _, err := manager.Parse(tampered, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("변조된 토큰을 허용했습니다: %v", err)
	}
}
