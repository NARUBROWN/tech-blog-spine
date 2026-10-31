package security

import (
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
