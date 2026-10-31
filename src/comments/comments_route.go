package comments

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewCommentsRepository,
		NewCommentsService,
		NewCommentsController,
	)
	app.Route("GET", "/api/v1/comments/post/:id", (*CommentsController).List)
	app.Route("POST", "/api/v1/comments/post/:id", (*CommentsController).Create)
	app.Route("PATCH", "/api/v1/comments/:id", (*CommentsController).Update)
	app.Route("DELETE", "/api/v1/comments/:id", (*CommentsController).Delete)
	return nil
}
