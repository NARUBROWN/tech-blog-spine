package comments

import (
	"context"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
)

type CommentsController struct{ service *CommentsService }

func NewCommentsController(s *CommentsService) *CommentsController { return &CommentsController{s} }
func (c *CommentsController) List(ctx context.Context, id path.Int, q query.Values, h header.Values) (httpx.Response[CommentPage], error) {
	_, cookies, err := c.service.Visitor(h, false)
	if err != nil {
		return httpx.Response[CommentPage]{}, err
	}
	out, err := c.service.List(ctx, id.Value, int(q.Int("page", 0)))
	return httpx.Response[CommentPage]{Body: out, Options: httpx.ResponseOptions{Cookies: cookies, Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
func (c *CommentsController) Create(ctx context.Context, id path.Int, r *CommentRequest, h header.Values) error {
	v, _, err := c.service.Visitor(h, true)
	if err != nil {
		return err
	}
	return c.service.Create(ctx, v, id.Value, *r)
}
func (c *CommentsController) Update(ctx context.Context, id path.Int, r *CommentRequest, h header.Values) error {
	v, _, err := c.service.Visitor(h, true)
	if err != nil {
		return err
	}
	return c.service.Change(ctx, v, id.Value, *r, false)
}
func (c *CommentsController) Delete(ctx context.Context, id path.Int, r *CommentRequest, h header.Values) error {
	v, _, err := c.service.Visitor(h, true)
	if err != nil {
		return err
	}
	return c.service.Change(ctx, v, id.Value, *r, true)
}
