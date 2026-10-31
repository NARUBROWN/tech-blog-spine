package likes

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewLikesController,
	)
	app.Route("POST", "/api/v1/like/:postId", (*LikesController).Like)
	app.Route("DELETE", "/api/v1/like/:postId", (*LikesController).Unlike)
	app.Route("GET", "/api/v1/like/:postId", (*LikesController).Users)
	return nil
}
