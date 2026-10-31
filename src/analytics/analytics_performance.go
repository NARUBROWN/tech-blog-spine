package analytics

import (
	"context"
	"math"
)

type PerformanceSample struct {
	ID         string  `json:"id"`
	Metric     string  `json:"metric"`
	Endpoint   string  `json:"endpoint"`
	DurationMs float64 `json:"durationMs"`
	StatusCode int     `json:"statusCode"`
	Revision   int     `json:"revision"`
}

// 검색어, 게시글 slug와 식별자가 포함된 원본 URL은 저장하지 않습니다.
func validEndpoint(endpoint string) bool {
	switch endpoint {
	case "/api/v1/post", "/api/v1/post/search/recent", "/api/v1/post/id/:id", "/api/v1/post/slug/:slug", "/api/v1/category/list", "/api/v1/comments/post/:id", "/api/v1/like/:id/state":
		return true
	}
	return false
}

func validatePerformance(s PerformanceSample) error {
	if !uuidPattern.MatchString(s.ID) || math.IsNaN(s.DurationMs) || math.IsInf(s.DurationMs, 0) || s.DurationMs < 0 || s.DurationMs > 600000 || s.Revision < 1 || s.Revision > 1000000 {
		return bad(400, "성능 측정값이 올바르지 않습니다.")
	}
	switch s.Metric {
	case "server_response", "api_response":
		if !validEndpoint(s.Endpoint) || s.StatusCode < 100 || s.StatusCode > 599 {
			return bad(400, "API 성능 경로가 올바르지 않습니다.")
		}
	case "fcp", "lcp", "inp", "page_ready", "route_ready":
		if s.Endpoint != "" || s.StatusCode != 0 {
			return bad(400, "화면 성능 표본에 API 정보가 포함되어 있습니다.")
		}
	default:
		return bad(400, "지원하지 않는 성능 지표입니다.")
	}
	return nil
}

type PerformanceStats struct {
	Metric    string  `json:"metric"`
	Path      string  `json:"path"`
	Endpoint  string  `json:"endpoint"`
	Count     int64   `json:"count"`
	Errors    int64   `json:"errors"`
	AverageMs float64 `json:"averageMs"`
	P50Ms     float64 `json:"p50Ms"`
	P95Ms     float64 `json:"p95Ms"`
	P99Ms     float64 `json:"p99Ms"`
	MaxMs     float64 `json:"maxMs"`
}

type PerformanceDashboard struct {
	Summary   []PerformanceStats `json:"summary"`
	Pages     []PerformanceStats `json:"pages"`
	Endpoints []PerformanceStats `json:"endpoints"`
}

// MySQL 8의 윈도 함수로 전체 원시 표본에서 nearest-rank 백분위를 계산합니다.
// 일별 백분위를 다시 평균하거나 표본을 일부만 잘라 계산하지 않습니다.
const performanceQuery = `WITH observations AS (
 SELECT p.metric,v.page_path,p.endpoint,p.duration_ms,p.status_code
 FROM analytics_performance p JOIN analytics_visit v ON v.id=p.visit_id
 WHERE p.created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY)
), scoped AS (
 SELECT 'summary' AS scope,metric,'' AS page_path,'' AS endpoint,duration_ms,status_code FROM observations
 UNION ALL
 SELECT 'page',metric,page_path,'',duration_ms,status_code FROM observations WHERE endpoint=''
 UNION ALL
 SELECT 'endpoint',metric,'',endpoint,duration_ms,status_code FROM observations WHERE endpoint<>''
), ranked AS (
 SELECT *,ROW_NUMBER() OVER (PARTITION BY scope,metric,page_path,endpoint ORDER BY duration_ms) AS rn,
 COUNT(*) OVER (PARTITION BY scope,metric,page_path,endpoint) AS total FROM scoped
)
SELECT scope,metric,page_path,endpoint,COUNT(*),SUM(status_code>=400),AVG(duration_ms),
 MIN(CASE WHEN rn>=CEIL(total*0.50) THEN duration_ms END),
 MIN(CASE WHEN rn>=CEIL(total*0.95) THEN duration_ms END),
 MIN(CASE WHEN rn>=CEIL(total*0.99) THEN duration_ms END),MAX(duration_ms)
FROM ranked GROUP BY scope,metric,page_path,endpoint ORDER BY scope,metric,page_path,endpoint`

func (s *AnalyticsService) Performance(ctx context.Context, days int) (PerformanceDashboard, error) {
	out := PerformanceDashboard{Summary: []PerformanceStats{}, Pages: []PerformanceStats{}, Endpoints: []PerformanceStats{}}
	if err := checkDays(days); err != nil {
		return out, err
	}
	rows, err := s.repository.db.QueryContext(ctx, performanceQuery, days-1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var scope string
		var r PerformanceStats
		if err := rows.Scan(&scope, &r.Metric, &r.Path, &r.Endpoint, &r.Count, &r.Errors, &r.AverageMs, &r.P50Ms, &r.P95Ms, &r.P99Ms, &r.MaxMs); err != nil {
			return out, err
		}
		switch scope {
		case "summary":
			out.Summary = append(out.Summary, r)
		case "page":
			out.Pages = append(out.Pages, r)
		case "endpoint":
			out.Endpoints = append(out.Endpoints, r)
		}
	}
	return out, rows.Err()
}
