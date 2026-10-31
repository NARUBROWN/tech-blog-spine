package analytics

import (
	"context"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/query"
	spinecontext "github.com/NARUBROWN/spine/pkg/spine"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type AnalyticsController struct{ service *AnalyticsService }

func NewAnalyticsController(s *AnalyticsService) *AnalyticsController { return &AnalyticsController{s} }
func (c *AnalyticsController) Start(ctx context.Context, r *VisitRequest, h header.Values) (httpx.Response[VisitResponse], error) {
	out, cookies, err := c.service.Start(ctx, h, *r)
	return httpx.Response[VisitResponse]{Body: out, Options: httpx.ResponseOptions{Cookies: cookies, Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
func (c *AnalyticsController) Collect(ctx context.Context, r *BatchRequest, h header.Values) error {
	return c.service.Collect(ctx, h, *r)
}
func (c *AnalyticsController) Interest(ctx context.Context) (httpx.Response[map[string]int64], error) {
	n, err := c.service.Interest(ctx)
	return httpx.Response[map[string]int64]{Body: map[string]int64{"count": n}, Options: httpx.ResponseOptions{Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
func (c *AnalyticsController) Dashboard(ctx context.Context, q query.Values, sc spinecontext.Ctx) (httpx.Response[Dashboard], error) {
	id, err := security.IdentityFromContext(sc)
	if err != nil {
		return httpx.Response[Dashboard]{}, err
	}
	if err = c.service.Admin(ctx, id); err != nil {
		return httpx.Response[Dashboard]{}, err
	}
	out, err := c.service.Dashboard(ctx, int(q.Int("days", 30)))
	return httpx.Response[Dashboard]{Body: out, Options: httpx.ResponseOptions{Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
func (c *AnalyticsController) Events(ctx context.Context, q query.Values, sc spinecontext.Ctx) (httpx.Response[EventPage], error) {
	id, err := security.IdentityFromContext(sc)
	if err != nil {
		return httpx.Response[EventPage]{}, err
	}
	if err = c.service.Admin(ctx, id); err != nil {
		return httpx.Response[EventPage]{}, err
	}
	out, err := c.service.Events(ctx, int(q.Int("days", 30)), int(q.Int("page", 0)), q.Get("type"))
	return httpx.Response[EventPage]{Body: out, Options: httpx.ResponseOptions{Headers: map[string]string{"Cache-Control": "no-store"}}}, err
}
