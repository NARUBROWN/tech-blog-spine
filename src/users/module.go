package users

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewUsersController,
	)
	app.Route("POST", "/api/v1/user/author-signup", (*UsersController).AuthorSignup)
	app.Route("POST", "/api/v1/user/admin-signup", (*UsersController).AdminSignup)
	app.Route("POST", "/api/v1/user/normal-signup", (*UsersController).NormalSignup)
	app.Route("PATCH", "/api/v1/user/:id", (*UsersController).Update)
	return nil
}
