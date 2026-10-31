package auth

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewAuthController,
	)
	app.Route("POST", "/api/v1/auth/login", (*AuthController).Login)
	app.Route("POST", "/api/v1/auth/logout", (*AuthController).Logout)
	return nil
}
