package posts

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewPostsController,
	)
	app.Route("POST", "/api/v1/post", (*PostsController).Create)
	app.Route("PATCH", "/api/v1/post", (*PostsController).Modify)
	app.Route("GET", "/api/v1/post/slug/:slug", (*PostsController).GetBySlug)
	app.Route("GET", "/api/v1/post/id/:id", (*PostsController).GetByID)
	app.Route("DELETE", "/api/v1/post/:id", (*PostsController).Delete)
	app.Route("POST", "/api/v1/post/view-count/:id", (*PostsController).IncrementViewCount)
	app.Route("GET", "/api/v1/post", (*PostsController).List)
	app.Route("GET", "/api/v1/post/search/recent", (*PostsController).SearchRecent)
	return nil
}
