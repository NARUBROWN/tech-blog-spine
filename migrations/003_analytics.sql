CREATE TABLE IF NOT EXISTS analytics_visit (
 id CHAR(36) NOT NULL PRIMARY KEY,
 visitor_hash CHAR(64) NOT NULL,
 session_hash CHAR(64) NOT NULL,
 page_path VARCHAR(512) NOT NULL,
 page_kind VARCHAR(16) NOT NULL,
 post_id BIGINT NULL,
 referrer_host VARCHAR(255) NOT NULL DEFAULT '',
 device VARCHAR(12) NOT NULL,
 active_ms BIGINT NOT NULL DEFAULT 0,
 scroll_depth INT NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 KEY idx_analytics_date (created_at),
 KEY idx_analytics_post (post_id,created_at),
 KEY idx_analytics_visitor (visitor_hash,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS analytics_event (
 id CHAR(36) NOT NULL PRIMARY KEY,
 visit_id CHAR(36) NOT NULL,
 event_type VARCHAR(32) NOT NULL,
 label VARCHAR(120) NOT NULL DEFAULT '',
 target_url VARCHAR(512) NOT NULL DEFAULT '',
 selected_text VARCHAR(1000) NOT NULL DEFAULT '',
 context_text VARCHAR(1500) NOT NULL DEFAULT '',
 section_name VARCHAR(80) NOT NULL DEFAULT '',
 duration_ms INT NOT NULL DEFAULT 0,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 KEY idx_analytics_event_date (created_at),
 KEY idx_analytics_event_type (event_type,created_at),
 FOREIGN KEY (visit_id) REFERENCES analytics_visit(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS analytics_interest (
 visitor_hash CHAR(64) NOT NULL PRIMARY KEY,
 first_seen DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
