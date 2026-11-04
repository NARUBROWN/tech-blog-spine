package security

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

type fakeRefreshStore struct {
	values map[int64]map[string]string
}

func (s *fakeRefreshStore) Save(_ context.Context, userID int64, token string, _ time.Duration) error {
	if s.values[userID] == nil {
		s.values[userID] = make(map[string]string)
	}
	s.values[userID][token] = token
	return nil
}

func (s *fakeRefreshStore) Get(_ context.Context, userID int64, token string) (string, error) {
	return s.values[userID][token], nil
}

func (s *fakeRefreshStore) Delete(_ context.Context, userID int64, token string) error {
	delete(s.values[userID], token)
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
	store := &fakeRefreshStore{values: map[int64]map[string]string{7: {refreshToken: refreshToken}}}
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
	interceptor := &AuthInterceptor{tokens: manager, store: &fakeRefreshStore{values: map[int64]map[string]string{}}}

	if err := interceptor.PreHandle(ctx, core.HandlerMeta{}); err == nil {
		t.Fatal("인증 쿠키가 없는 보호 경로를 허용했습니다")
	}
	if len(writer.headers["Set-Cookie"]) != 2 {
		t.Fatalf("무효 쿠키 삭제 응답이 없습니다: %v", writer.headers)
	}
}

func TestPrivilegedSignupRequiresAuthentication(t *testing.T) {
	interceptor := &AuthInterceptor{tokens: NewTokenManager(config.Config{JWTSecret: "test-secret-with-at-least-32-bytes", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}), store: &fakeRefreshStore{values: map[int64]map[string]string{}}}
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

func TestAuthInterceptorKeepsOtherBrowserSessions(t *testing.T) {
	issuedAt := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	manager := NewTokenManager(config.Config{
		JWTSecret:      "a-test-secret-that-is-longer-than-32-bytes",
		AccessTokenTTL: 30 * time.Minute, RefreshTokenTTL: 7 * 24 * time.Hour,
	})
	manager.now = func() time.Time { return issuedAt }
	store := &fakeRefreshStore{values: make(map[int64]map[string]string)}
	var sessions []string
	for range 2 {
		token, err := manager.CreateRefreshToken(7, "tester")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Save(context.Background(), 7, token, manager.RefreshTTL()); err != nil {
			t.Fatal(err)
		}
		sessions = append(sessions, token)
	}
	unknownToken, err := manager.CreateRefreshToken(7, "tester")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return issuedAt.Add(31 * time.Minute) }
	interceptor := &AuthInterceptor{tokens: manager, store: store}
	for _, test := range []struct {
		name   string
		token  string
		status int
	}{
		{"첫 번째 브라우저", sessions[0], http.StatusOK},
		{"두 번째 브라우저", sessions[1], http.StatusOK},
		{"저장되지 않은 토큰", unknownToken, http.StatusUnauthorized},
		{"실패 후 첫 번째 브라우저", sessions[0], http.StatusOK},
		{"실패 후 두 번째 브라우저", sessions[1], http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &fakeResponseWriter{headers: make(map[string][]string)}
			ctx := &fakeExecutionContext{
				method: http.MethodPost, path: "/api/v1/post",
				headers: map[string]string{"Cookie": "refreshToken=" + test.token},
				values:  map[string]any{"spine.response_writer": writer},
			}
			err := interceptor.PreHandle(ctx, core.HandlerMeta{})
			if test.status == http.StatusUnauthorized {
				var httpError *httperr.HTTPError
				if !errors.As(err, &httpError) || httpError.Status != test.status {
					t.Fatalf("미등록 토큰을 거부하지 않았습니다: %v", err)
				}
				if len(writer.headers["Set-Cookie"]) != 2 {
					t.Fatal("실패한 브라우저의 쿠키가 삭제되지 않았습니다")
				}
				return
			}
			if err != nil {
				t.Fatalf("다른 브라우저 때문에 갱신이 실패했습니다: %v", err)
			}
			if identity, err := IdentityFromContext(ctx); err != nil || identity.UserID != 7 {
				t.Fatalf("인증 주체가 올바르지 않습니다: %+v, %v", identity, err)
			}
			if cookies := writer.headers["Set-Cookie"]; len(cookies) != 1 || !strings.HasPrefix(cookies[0], "accessToken=") {
				t.Fatal("새 액세스 토큰이 발급되지 않았습니다")
			}
		})
	}
}
