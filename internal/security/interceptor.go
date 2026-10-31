package security

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

const identityContextKey = "tech-blog.identity"

type Identity struct {
	UserID   int64
	Username string
}

type contextReader interface {
	Get(string) (any, bool)
}

func IdentityFromContext(ctx contextReader) (Identity, error) {
	value, ok := ctx.Get(identityContextKey)
	if !ok {
		return Identity{}, httperr.Unauthorized("인증이 필요합니다.")
	}
	identity, ok := value.(Identity)
	if !ok || identity.UserID <= 0 || identity.Username == "" {
		return Identity{}, httperr.Unauthorized("인증 정보가 올바르지 않습니다.")
	}
	return identity, nil
}

type AuthInterceptor struct {
	tokens *TokenManager
	store  RefreshTokenStore
}

func NewAuthInterceptor(tokens *TokenManager, store *RedisRefreshTokenStore) *AuthInterceptor {
	return &AuthInterceptor{tokens: tokens, store: store}
}

func (i *AuthInterceptor) PreHandle(ctx core.ExecutionContext, _ core.HandlerMeta) error {
	if isPublicRoute(ctx.Method(), ctx.Path()) {
		return nil
	}
	accessToken := requestCookie(ctx.Header("Cookie"), "accessToken")
	if claims, err := i.tokens.Parse(accessToken, AccessToken); err == nil {
		ctx.Set(identityContextKey, Identity{UserID: claims.UserID, Username: claims.Subject})
		return nil
	}

	refreshToken := requestCookie(ctx.Header("Cookie"), "refreshToken")
	claims, err := i.tokens.Parse(refreshToken, RefreshToken)
	if err != nil {
		i.addCookies(ctx, i.tokens.ClearCookies())
		return httperr.Unauthorized("인증이 필요합니다.")
	}
	stored, err := i.store.Get(ctx.Context(), claims.UserID)
	if err != nil {
		return httperr.InternalServerError("인증 저장소를 확인할 수 없습니다.")
	}
	if len(stored) != len(refreshToken) || subtle.ConstantTimeCompare([]byte(stored), []byte(refreshToken)) != 1 {
		_ = i.store.Delete(ctx.Context(), claims.UserID)
		i.addCookies(ctx, i.tokens.ClearCookies())
		return httperr.Unauthorized("인증이 필요합니다.")
	}
	newAccessToken, err := i.tokens.CreateAccessToken(claims.UserID, claims.Subject)
	if err != nil {
		return httperr.InternalServerError("액세스 토큰을 발급할 수 없습니다.")
	}
	i.addCookies(ctx, []httpx.Cookie{i.tokens.AccessCookie(newAccessToken)})
	ctx.Set(identityContextKey, Identity{UserID: claims.UserID, Username: claims.Subject})
	return nil
}

func (*AuthInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}

func (*AuthInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}

func (*AuthInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}

func (i *AuthInterceptor) addCookies(ctx core.ExecutionContext, cookies []httpx.Cookie) {
	writerValue, ok := ctx.Get("spine.response_writer")
	if !ok {
		return
	}
	writer, ok := writerValue.(core.ResponseWriter)
	if !ok {
		return
	}
	for _, cookie := range cookies {
		sameSite := http.SameSiteDefaultMode
		switch cookie.SameSite {
		case httpx.SameSiteLax:
			sameSite = http.SameSiteLaxMode
		case httpx.SameSiteStrict:
			sameSite = http.SameSiteStrictMode
		case httpx.SameSiteNone:
			sameSite = http.SameSiteNoneMode
		}
		writer.AddHeader("Set-Cookie", (&http.Cookie{
			Name: cookie.Name, Value: cookie.Value, Path: cookie.Path, Domain: cookie.Domain,
			MaxAge: cookie.MaxAge, Expires: expiry(cookie.Expires), HttpOnly: cookie.HttpOnly,
			Secure: cookie.Secure, SameSite: sameSite,
		}).String())
	}
}

func expiry(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func requestCookie(raw, name string) string {
	request := http.Request{Header: http.Header{"Cookie": []string{raw}}}
	cookie, err := request.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func CookieFromHeader(raw, name string) string {
	return requestCookie(raw, name)
}

func isPublicRoute(method, path string) bool {
	if strings.HasPrefix(path, "/api/v1/comments/") || (strings.HasPrefix(path, "/api/v1/post/") && strings.HasSuffix(path, "/comments")) || strings.HasPrefix(path, "/api/v1/like/") {
		return true
	}
	if method == http.MethodOptions || path == "/health" {
		return true
	}
	if method == http.MethodPost && (path == "/api/v1/auth/login" || path == "/api/v1/auth/logout") {
		return true
	}
	if method == http.MethodPost && path == "/api/v1/user/normal-signup" {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/image") {
		return true
	}
	if method == http.MethodGet && (path == "/api/v1/post" || strings.HasPrefix(path, "/api/v1/post/")) {
		return true
	}
	if method == http.MethodPost && strings.HasPrefix(path, "/api/v1/post/view-count/") {
		return true
	}
	if method == http.MethodGet && path == "/api/v1/category/list" {
		return true
	}
	return method == http.MethodGet && strings.HasPrefix(path, "/api/v1/like/")
}
