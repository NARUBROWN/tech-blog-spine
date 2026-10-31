package auth

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type AuthController struct{ service *blog.Service }

func NewAuthController(service *blog.Service) *AuthController {
	return &AuthController{service: service}
}

func (c *AuthController) Login(ctx context.Context, request *model.LoginRequest) (httpx.Response[model.LoginResponse], error) {
	response, cookies, err := c.service.Login(ctx, *request)
	return httpx.Response[model.LoginResponse]{Body: response, Options: httpx.ResponseOptions{Cookies: cookies}}, err
}

func (c *AuthController) Logout(ctx context.Context, headers header.Values) httpx.Response[map[string]string] {
	cookies := c.service.Logout(ctx, security.CookieFromHeader(headers.Get("Cookie"), "refreshToken"))
	return httpx.Response[map[string]string]{
		Body:    map[string]string{"status": "ok"},
		Options: httpx.ResponseOptions{Cookies: cookies},
	}
}
