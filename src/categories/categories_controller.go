package categories

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/query"
	spinecontext "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type CategoriesController struct{ service *blog.Service }

func NewCategoriesController(service *blog.Service) *CategoriesController {
	return &CategoriesController{service: service}
}

func (c *CategoriesController) Create(ctx context.Context, values query.Values, spineCtx spinecontext.Ctx) error {
	identity, err := security.IdentityFromContext(spineCtx)
	if err != nil {
		return err
	}
	return c.service.CreateCategory(ctx, identity, values.Get("name"))
}

func (c *CategoriesController) List(ctx context.Context) (httpx.Response[[]model.Category], error) {
	response, err := c.service.ListCategories(ctx)
	return httpx.Response[[]model.Category]{Body: response}, err
}
