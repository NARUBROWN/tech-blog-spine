package security

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

type fakeRefreshStore struct {
	values map[int64]string
}

func (s *fakeRefreshStore) Save(_ context.Context, userID int64, token string, _ time.Duration) error {
	s.values[userID] = token
	return nil
}

func (s *fakeRefreshStore) Get(_ context.Context, userID int64) (string, error) {
	return s.values[userID], nil
}

func (s *fakeRefreshStore) Delete(_ context.Context, userID int64) error {
	delete(s.values, userID)
	return nil
}

type fakeExecutionContext struct {
	method  string
	path    string
	headers map[string]string
	values  map[string]any
}

func (c *fakeExecutionContext) Context() context.Context     { return context.Background() }
func (c *fakeExecutionContext) EventBus() core.EventBus      { return nil }
func (c *fakeExecutionContext) Method() string               { return c.method }
func (c *fakeExecutionContext) Path() string                 { return c.path }
func (c *fakeExecutionContext) Params() map[string]string    { return nil }
func (c *fakeExecutionContext) Header(name string) string    { return c.headers[name] }
func (c *fakeExecutionContext) PathKeys() []string           { return nil }
func (c *fakeExecutionContext) Queries() map[string][]string { return nil }
func (c *fakeExecutionContext) Set(key string, value any)    { c.values[key] = value }
func (c *fakeExecutionContext) Get(key string) (any, bool) {
	value, ok := c.values[key]
	return value, ok
}

type fakeResponseWriter struct {
	headers map[string][]string
}

func (w *fakeResponseWriter) SetHeader(key, value string) { w.headers[key] = []string{value} }
func (w *fakeResponseWriter) AddHeader(key, value string) {
	w.headers[key] = append(w.headers[key], value)
}
func (*fakeResponseWriter) IsCommitted() bool             { return false }
func (*fakeResponseWriter) WriteStatus(int) error         { return nil }
func (*fakeResponseWriter) WriteJSON(int, any) error      { return nil }
func (*fakeResponseWriter) WriteString(int, string) error { return nil }
func (*fakeResponseWriter) WriteBytes(int, []byte) error  { return nil }

func TestAuthInterceptorReissuesAccessTokenFromRefreshToken(t *testing.T) {
	issuedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	manager := NewTokenManager(config.Config{
		JWTSecret:       "a-test-secret-that-is-longer-than-32-bytes",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	manager.now = func() time.Time { return issuedAt }
	accessToken, _ := manager.CreateAccessToken(7, "tester")
	refreshToken, _ := manager.CreateRefreshToken(7, "tester")
	manager.now = func() time.Time { return issuedAt.Add(2 * time.Minute) }
	store := &fakeRefreshStore{values: map[int64]string{7: refreshToken}}
	interceptor := &AuthInterceptor{tokens: manager, store: store}
	writer := &fakeResponseWriter{headers: make(map[string][]string)}
	ctx := &fakeExecutionContext{
		method: http.MethodPost, path: "/api/v1/post",
		headers: map[string]string{"Cookie": "accessToken=" + accessToken + "; refreshToken=" + refreshToken},
		values:  map[string]any{"spine.response_writer": writer},
	}

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err != nil {
		t.Fatalf("리프레시 인증 실패: %v", err)
	}
	identity, err := IdentityFromContext(ctx)
	if err != nil || identity.UserID != 7 {
		t.Fatalf("인증 주체가 설정되지 않았습니다: %+v, %v", identity, err)
	}
	setCookies := writer.headers["Set-Cookie"]
	if len(setCookies) != 1 || !strings.HasPrefix(setCookies[0], "accessToken=") {
		t.Fatalf("새 액세스 쿠키가 없습니다: %v", setCookies)
	}
}

func TestAuthInterceptorClearsInvalidCookies(t *testing.T) {
	manager := NewTokenManager(config.Config{
		JWTSecret:       "a-test-secret-that-is-longer-than-32-bytes",
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
	})
	writer := &fakeResponseWriter{headers: make(map[string][]string)}
	ctx := &fakeExecutionContext{
		method: http.MethodDelete, path: "/api/v1/post/1",
		headers: map[string]string{}, values: map[string]any{"spine.response_writer": writer},
	}
	interceptor := &AuthInterceptor{tokens: manager, store: &fakeRefreshStore{values: map[int64]string{}}}

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err == nil {
		t.Fatal("인증 쿠키가 없는 보호 경로를 허용했습니다")
	}
	if len(writer.headers["Set-Cookie"]) != 2 {
		t.Fatalf("무효 쿠키 삭제 응답이 없습니다: %v", writer.headers)
	}
}

func TestPrivilegedSignupRequiresAuthentication(t *testing.T) {
	interceptor := &AuthInterceptor{tokens: NewTokenManager(config.Config{JWTSecret: "test-secret-with-at-least-32-bytes", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}), store: &fakeRefreshStore{values: map[int64]string{}}}
	for _, path := range []string{"/api/v1/user/admin-signup", "/api/v1/user/author-signup", "/api/v1/post", "/api/v1/category"} {
		ctx := &fakeExecutionContext{method: http.MethodPost, path: path, headers: map[string]string{}, values: map[string]any{}}
		if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err == nil {
			t.Fatalf("anonymous privilege path allowed: %s", path)
		}
	}
	if !isPublicRoute(http.MethodPost, "/api/v1/user/normal-signup") || !isPublicRoute(http.MethodGet, "/api/v1/post") {
		t.Fatal("public flow blocked")
	}
}
