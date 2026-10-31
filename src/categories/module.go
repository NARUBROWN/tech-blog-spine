package categories

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewCategoriesController,
	)
	app.Route("POST", "/api/v1/category", (*CategoriesController).Create)
	app.Route("GET", "/api/v1/category/list", (*CategoriesController).List)
	return nil
}
