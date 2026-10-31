package images

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewImagesController,
	)
	app.Route("POST", "/api/v1/image", (*ImagesController).Upload)
	app.Route("GET", "/api/v1/image/:uuid", (*ImagesController).Get)
	return nil
}
