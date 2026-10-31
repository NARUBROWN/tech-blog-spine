package likes

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
	spinecontext "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type LikesController struct{ service *blog.Service }

func NewLikesController(service *blog.Service) *LikesController {
	return &LikesController{service: service}
}

func (c *LikesController) Like(ctx context.Context, postID path.Int, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.LikePost(ctx, postID.Value, identity.UserID)
}

func (c *LikesController) Unlike(ctx context.Context, postID path.Int, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.UnlikePost(ctx, postID.Value, identity.UserID)
}

func (c *LikesController) Users(ctx context.Context, postID path.Int) (httpx.Response[[]model.UserResponse], error) {
	response, err := c.service.UsersWhoLiked(ctx, postID.Value)
	return httpx.Response[[]model.UserResponse]{Body: response}, err
}
