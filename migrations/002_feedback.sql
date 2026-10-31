CREATE TABLE IF NOT EXISTS post_comment (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 post_id BIGINT NOT NULL,
 nickname VARCHAR(24) NOT NULL,
 content TEXT NOT NULL,
 password_hash VARCHAR(100) NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
 KEY idx_comment_post (post_id,id),
 FOREIGN KEY (post_id) REFERENCES post(post_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS post_reaction (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 post_id BIGINT NOT NULL,
 user_id BIGINT NULL,
 visitor_hash CHAR(64) NULL,
 active BOOLEAN NOT NULL DEFAULT TRUE,
 UNIQUE KEY uk_reaction_user (post_id,user_id),
 UNIQUE KEY uk_reaction_visitor (post_id,visitor_hash),
 FOREIGN KEY (post_id) REFERENCES post(post_id) ON DELETE CASCADE,
 FOREIGN KEY (user_id) REFERENCES user(user_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS feedback_rate (
 network_hash CHAR(64) NOT NULL,
 action VARCHAR(20) NOT NULL,
 window_start BIGINT NOT NULL,
 requests INT NOT NULL DEFAULT 1,
 PRIMARY KEY (network_hash,action,window_start),
 KEY idx_feedback_rate_expiry (window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
