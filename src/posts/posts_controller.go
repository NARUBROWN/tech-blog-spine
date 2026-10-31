package posts

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
	spinecontext "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type PostsController struct{ service *blog.Service }

func NewPostsController(service *blog.Service) *PostsController {
	return &PostsController{service: service}
}

func (c *PostsController) Create(ctx context.Context, request *model.PostRequest, values query.Values, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.CreatePost(ctx, identity, values.Int("categoryId", 0), *request)
}

func (c *PostsController) Modify(ctx context.Context, request *model.PostRequest, values query.Values, spineCtx spinecontext.Ctx) (httpx.Response[model.PostEditResponse], error) {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return httpx.Response[model.PostEditResponse]{}, err
	}
	response, err := c.service.UpdatePost(ctx, identity, values.Int("postId", 0), values.Int("categoryId", 0), *request)
	return httpx.Response[model.PostEditResponse]{Body: response}, err
}

func (c *PostsController) GetBySlug(ctx context.Context, slug path.String) (httpx.Response[model.PostResponse], error) {
	response, err := c.service.PostBySlug(ctx, slug.Value)
	return httpx.Response[model.PostResponse]{Body: response}, err
}

func (c *PostsController) GetByID(ctx context.Context, id path.Int) (httpx.Response[model.PostResponse], error) {
	response, err := c.service.PostByID(ctx, id.Value)
	return httpx.Response[model.PostResponse]{Body: response}, err
}

func (c *PostsController) Delete(ctx context.Context, id path.Int, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.DeletePost(ctx, identity, id.Value)
}

func (c *PostsController) IncrementViewCount(ctx context.Context, id path.Int) error {
	return c.service.IncrementViewCount(ctx, id.Value)
}

func (c *PostsController) List(ctx context.Context, values query.Values) (httpx.Response[model.PageResponse[model.PostResponse]], error) {
	response, err := c.service.ListPosts(
		ctx, values.Get("categoryName"), int(values.Int("page", 0)), int(values.Int("size", 20)),
	)
	return httpx.Response[model.PageResponse[model.PostResponse]]{Body: response}, err
}

func (c *PostsController) SearchRecent(ctx context.Context, values query.Values) (httpx.Response[model.PageResponse[model.PostResponse]], error) {
	response, err := c.service.SearchPosts(ctx, values.Get("keyword"), int(values.Int("page", 0)), int(values.Int("size", 20)))
	return httpx.Response[model.PageResponse[model.PostResponse]]{Body: response}, err
}
