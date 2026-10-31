package users

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
	spinecontext "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type UsersController struct{ service *blog.Service }

func NewUsersController(service *blog.Service) *UsersController {
	return &UsersController{service: service}
}

func (c *UsersController) AuthorSignup(ctx context.Context, request *model.UserRequest, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.CreatePrivilegedUser(ctx, identity, *request, model.RoleAuthor)
}

func (c *UsersController) AdminSignup(ctx context.Context, request *model.UserRequest, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.CreatePrivilegedUser(ctx, identity, *request, model.RoleAdmin)
}

func (c *UsersController) NormalSignup(ctx context.Context, request *model.UserRequest) error {
	return c.service.CreateUser(ctx, *request, model.RoleUser)
}

func (c *UsersController) Update(ctx context.Context, id path.Int, request *model.UserRequest, spineCtx spinecontext.Ctx) (httpx.Response[model.UserResponse], error) {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return httpx.Response[model.UserResponse]{}, err
	}
	response, err := c.service.UpdateUser(ctx, identity, id.Value, *request)
	return httpx.Response[model.UserResponse]{Body: response}, err
}
