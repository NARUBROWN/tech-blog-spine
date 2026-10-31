package likes

import (
	"context"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/src/comments"
)

type LikesController struct{ service *comments.CommentsService }

func NewLikesController(s *comments.CommentsService) *LikesController { return &LikesController{s} }
func (c *LikesController) Like(ctx context.Context, id path.Int, h header.Values) (httpx.Response[comments.ReactionState], error) {
	v, _, err := c.service.Visitor(h, true)
	if err != nil {
		return httpx.Response[comments.ReactionState]{}, err
	}
	out, err := c.service.React(ctx, v, id.Value, true)
	return httpx.Response[comments.ReactionState]{Body: out}, err
}
func (c *LikesController) Unlike(ctx context.Context, id path.Int, h header.Values) (httpx.Response[comments.ReactionState], error) {
	v, _, err := c.service.Visitor(h, true)
	if err != nil {
		return httpx.Response[comments.ReactionState]{}, err
	}
	out, err := c.service.React(ctx, v, id.Value, false)
	return httpx.Response[comments.ReactionState]{Body: out}, err
}
func (c *LikesController) State(ctx context.Context, id path.Int, h header.Values) (httpx.Response[comments.ReactionState], error) {
	v, cookies, err := c.service.Visitor(h, false)
	if err != nil {
		return httpx.Response[comments.ReactionState]{}, err
	}
	out, err := c.service.Reactions(ctx, v, id.Value)
	return httpx.Response[comments.ReactionState]{Body: out, Options: httpx.ResponseOptions{Cookies: cookies, Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
func (c *LikesController) Users(ctx context.Context, id path.Int, h header.Values) (httpx.Response[[]model.UserResponse], error) {
	v, cookies, err := c.service.Visitor(h, false)
	if err != nil {
		return httpx.Response[[]model.UserResponse]{}, err
	}
	out, err := c.service.Reactions(ctx, v, id.Value)
	return httpx.Response[[]model.UserResponse]{Body: out.Users, Options: httpx.ResponseOptions{Cookies: cookies, Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
