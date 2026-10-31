package analytics

import "github.com/NARUBROWN/spine"

// Register는 이 resource를 애플리케이션에 등록합니다.
func Register(app spine.App) error {
	app.Constructor(
		NewAnalyticsRepository,
		NewAnalyticsService,
		NewAnalyticsController,
	)
	app.Route("POST", "/api/v1/analytics/visits", (*AnalyticsController).Start)
	app.Route("POST", "/api/v1/analytics/events", (*AnalyticsController).Collect)
	app.Route("GET", "/api/v1/analytics/interest", (*AnalyticsController).Interest)
	app.Route("GET", "/api/v1/admin/analytics", (*AnalyticsController).Dashboard)
	app.Route("GET", "/api/v1/admin/analytics/events", (*AnalyticsController).Events)
	return nil
}
