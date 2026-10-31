package images

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/tech-blog-spine/internal/blog"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
)

type ImagesController struct{ service *blog.Service }

func NewImagesController(service *blog.Service) *ImagesController {
	return &ImagesController{service: service}
}

func (c *ImagesController) Upload(ctx context.Context, files multipart.UploadedFiles) (httpx.Response[model.ImageUploadResponse], error) {
	response, err := c.service.UploadImage(ctx, files)
	return httpx.Response[model.ImageUploadResponse]{Body: response}, err
}

func (c *ImagesController) Get(ctx context.Context, publicID path.String) (httpx.Binary, error) {
	data, contentType, err := c.service.LoadImage(ctx, publicID.Value)
	return httpx.Binary{Data: data, ContentType: contentType}, err
}
