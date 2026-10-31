package analytics

import (
	"context"
	"time"
)

type Summary struct {
	Views           int64 `json:"views"`
	Visitors        int64 `json:"visitors"`
	Sessions        int64 `json:"sessions"`
	AverageActiveMs int64 `json:"averageActiveMs"`
	ResumeViews     int64 `json:"resumeViews"`
	ResumeVisitors  int64 `json:"resumeVisitors"`
	ResumeClicks    int64 `json:"resumeClicks"`
	LinkedInClicks  int64 `json:"linkedInClicks"`
	Selections      int64 `json:"selections"`
	OutboundClicks  int64 `json:"outboundClicks"`
}
type Daily struct {
	Date     string `json:"date"`
	Views    int64  `json:"views"`
	Visitors int64  `json:"visitors"`
}
type PageRow struct {
	Path            string `json:"path"`
	Title           string `json:"title"`
	Views           int64  `json:"views"`
	Visitors        int64  `json:"visitors"`
	AverageActiveMs int64  `json:"averageActiveMs"`
	AverageScroll   int64  `json:"averageScroll"`
}
type PostRow struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Views           int64  `json:"views"`
	LegacyViews     int64  `json:"legacyViews"`
	AverageActiveMs int64  `json:"averageActiveMs"`
}
type SourceRow struct {
	Name     string `json:"name"`
	Views    int64  `json:"views"`
	Visitors int64  `json:"visitors"`
}
type EventGroup struct {
	Type              string `json:"type"`
	Text              string `json:"text"`
	Context           string `json:"context"`
	Section           string `json:"section"`
	URL               string `json:"url"`
	Label             string `json:"label"`
	Count             int64  `json:"count"`
	Visitors          int64  `json:"visitors"`
	AverageDurationMs int64  `json:"averageDurationMs"`
}
type Dashboard struct {
	Summary       Summary      `json:"summary"`
	Daily         []Daily      `json:"daily"`
	Pages         []PageRow    `json:"pages"`
	Posts         []PostRow    `json:"posts"`
	Referrers     []SourceRow  `json:"referrers"`
	Devices       []SourceRow  `json:"devices"`
	Groups        []EventGroup `json:"groups"`
	RetentionDays int          `json:"retentionDays"`
}
type EventRow struct {
	ID         string `json:"id"`
	At         string `json:"at"`
	Type       string `json:"type"`
	Path       string `json:"path"`
	Visitor    string `json:"visitor"`
	Text       string `json:"text"`
	Context    string `json:"context"`
	Section    string `json:"section"`
	URL        string `json:"url"`
	Label      string `json:"label"`
	DurationMs int64  `json:"durationMs"`
}
type EventPage struct {
	Content       []EventRow `json:"content"`
	Number        int        `json:"number"`
	TotalPages    int        `json:"totalPages"`
	TotalElements int64      `json:"totalElements"`
}

func checkDays(days int) error {
	if days != 1 && days != 7 && days != 30 && days != 90 {
		return bad(400, "조회 기간은 1, 7, 30, 90일 중 선택해 주세요.")
	}
	return nil
}
func (s *AnalyticsService) Dashboard(ctx context.Context, days int) (Dashboard, error) {
	out := Dashboard{Daily: []Daily{}, Pages: []PageRow{}, Posts: []PostRow{}, Referrers: []SourceRow{}, Devices: []SourceRow{}, Groups: []EventGroup{}, RetentionDays: 90}
	if err := checkDays(days); err != nil {
		return out, err
	}
	db := s.repository.db
	age := days - 1
	err := db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(DISTINCT visitor_hash),COUNT(DISTINCT session_hash),COALESCE(ROUND(AVG(active_ms)),0),COALESCE(SUM(page_kind='about'),0),COUNT(DISTINCT CASE WHEN page_kind='about' THEN visitor_hash END) FROM analytics_visit WHERE created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY)`, age).Scan(&out.Summary.Views, &out.Summary.Visitors, &out.Summary.Sessions, &out.Summary.AverageActiveMs, &out.Summary.ResumeViews, &out.Summary.ResumeVisitors)
	if err != nil {
		return out, err
	}
	err = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(event_type='snackbar_resume'),0),COALESCE(SUM(event_type='snackbar_linkedin'),0),COALESCE(SUM(event_type='resume_selection'),0),COALESCE(SUM(event_type='resume_outbound'),0) FROM analytics_event WHERE created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY)`, age).Scan(&out.Summary.ResumeClicks, &out.Summary.LinkedInClicks, &out.Summary.Selections, &out.Summary.OutboundClicks)
	if err != nil {
		return out, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DATE_FORMAT(created_at,'%Y-%m-%d'),COUNT(*),COUNT(DISTINCT visitor_hash) FROM analytics_visit WHERE created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY) GROUP BY DATE_FORMAT(created_at,'%Y-%m-%d') ORDER BY DATE_FORMAT(created_at,'%Y-%m-%d')`, age)
	if err != nil {
		return out, err
	}
	byDate := map[string]Daily{}
	for rows.Next() {
		var r Daily
		if err = rows.Scan(&r.Date, &r.Views, &r.Visitors); err != nil {
			rows.Close()
			return out, err
		}
		byDate[r.Date] = r
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	seoul, _ := time.LoadLocation("Asia/Seoul")
	today := time.Now().In(seoul)
	for i := age; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		r := byDate[date]
		r.Date = date
		out.Daily = append(out.Daily, r)
	}
	rows, err = db.QueryContext(ctx, `SELECT v.page_path,COALESCE(p.title,''),COUNT(*),COUNT(DISTINCT v.visitor_hash),ROUND(AVG(v.active_ms)),ROUND(AVG(v.scroll_depth)) FROM analytics_visit v LEFT JOIN post p ON p.post_id=v.post_id WHERE v.created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY) GROUP BY v.page_path,p.title ORDER BY COUNT(*) DESC LIMIT 100`, age)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r PageRow
		if err = rows.Scan(&r.Path, &r.Title, &r.Views, &r.Visitors, &r.AverageActiveMs, &r.AverageScroll); err != nil {
			rows.Close()
			return out, err
		}
		out.Pages = append(out.Pages, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = db.QueryContext(ctx, `SELECT p.post_id,p.title,COUNT(v.id),COALESCE(p.view_count,0),COALESCE(ROUND(AVG(v.active_ms)),0) FROM post p LEFT JOIN analytics_visit v ON v.post_id=p.post_id AND v.created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY) GROUP BY p.post_id,p.title,p.view_count ORDER BY COUNT(v.id) DESC,p.post_id DESC LIMIT 100`, age)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r PostRow
		if err = rows.Scan(&r.ID, &r.Title, &r.Views, &r.LegacyViews, &r.AverageActiveMs); err != nil {
			rows.Close()
			return out, err
		}
		out.Posts = append(out.Posts, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, kind := range []string{"referrer_host", "device"} {
		// 선택 가능한 컬럼은 고정된 두 값으로 제한합니다.
		rows, err = db.QueryContext(ctx, `SELECT `+kind+`,COUNT(*),COUNT(DISTINCT visitor_hash) FROM analytics_visit WHERE created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY) GROUP BY `+kind+` ORDER BY COUNT(*) DESC LIMIT 30`, age)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var r SourceRow
			if err = rows.Scan(&r.Name, &r.Views, &r.Visitors); err != nil {
				rows.Close()
				return out, err
			}
			if kind == "device" {
				out.Devices = append(out.Devices, r)
			} else {
				out.Referrers = append(out.Referrers, r)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	rows, err = db.QueryContext(ctx, `SELECT e.event_type,e.selected_text,e.context_text,e.section_name,e.target_url,e.label,COUNT(*),COUNT(DISTINCT v.visitor_hash),ROUND(AVG(e.duration_ms)) FROM analytics_event e JOIN analytics_visit v ON v.id=e.visit_id WHERE e.created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY) GROUP BY e.event_type,e.selected_text,e.context_text,e.section_name,e.target_url,e.label ORDER BY COUNT(*) DESC LIMIT 100`, age)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r EventGroup
		if err = rows.Scan(&r.Type, &r.Text, &r.Context, &r.Section, &r.URL, &r.Label, &r.Count, &r.Visitors, &r.AverageDurationMs); err != nil {
			rows.Close()
			return out, err
		}
		out.Groups = append(out.Groups, r)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}
func (s *AnalyticsService) Events(ctx context.Context, days, page int, kind string) (EventPage, error) {
	out := EventPage{Content: []EventRow{}, Number: page}
	if err := checkDays(days); err != nil {
		return out, err
	}
	if page < 0 || page > 100000 {
		return out, bad(400, "페이지가 올바르지 않습니다.")
	}
	if kind != "" && !validType(kind) {
		return out, bad(400, "이벤트 종류가 올바르지 않습니다.")
	}
	where := ` WHERE e.created_at>=DATE_SUB(CURDATE(),INTERVAL ? DAY)`
	args := []any{days - 1}
	if kind != "" {
		where += ` AND e.event_type=?`
		args = append(args, kind)
	}
	if err := s.repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_event e`+where, args...).Scan(&out.TotalElements); err != nil {
		return out, err
	}
	out.TotalPages = int((out.TotalElements + 49) / 50)
	args = append(args, 50, page*50)
	rows, err := s.repository.db.QueryContext(ctx, `SELECT e.id,e.created_at,e.event_type,v.page_path,LEFT(v.visitor_hash,10),e.selected_text,e.context_text,e.section_name,e.target_url,e.label,e.duration_ms FROM analytics_event e JOIN analytics_visit v ON v.id=e.visit_id`+where+` ORDER BY e.created_at DESC,e.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r EventRow
		var at time.Time
		if err = rows.Scan(&r.ID, &at, &r.Type, &r.Path, &r.Visitor, &r.Text, &r.Context, &r.Section, &r.URL, &r.Label, &r.DurationMs); err != nil {
			return out, err
		}
		r.At = at.Format(time.RFC3339)
		out.Content = append(out.Content, r)
	}
	return out, rows.Err()
}
