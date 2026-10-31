# Tech Blog Spine API

기존 `tech-blog-springboot`의 HTTP 계약과 도메인 동작을 Go 기반 [Spine](https://github.com/NARUBROWN/spine) 애플리케이션으로 옮긴 프로젝트입니다. 프로젝트 골격과 리소스 모듈은 `spine-cli 0.1.1`의 `spine new`, `spine generate resource`로 생성했습니다.

## 이전된 기능

- 포스트 CRUD, 슬러그, SEO 필드, 조회수와 좋아요 카운터
- 카테고리와 태그 및 포스트-태그 관계
- HMAC-SHA256 JWT 액세스/리프레시 토큰과 HTTP-only 쿠키
- Redis 리프레시 토큰 저장 및 액세스 토큰 재발급
- MySQL 저장소와 트랜잭션 기반 관계 변경
- PNG/JPEG/WebP 형식·20MB 제한을 서버에서 검사하는 이미지 저장
- Spine CORS 및 인증 인터셉터
- Spine 정적 실행 계약, OpenAPI export, snapshot/diff

원본 Spring 프로젝트는 비교와 데이터 이전을 위해 상위 디렉터리의 `tech-blog-springboot`에 그대로 보존되어 있습니다.

## 빠른 실행

Go 1.25.5 이상, MySQL 8.4, Redis 7 이상이 필요합니다.

```bash
cp .env.example .env
# .env의 MYSQL_PASSWORD와 32바이트 이상 JWT_SECRET을 변경
set -a; source .env; set +a
mysql -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" "$MYSQL_DATABASE" < migrations/001_init.sql
go run .
```

전체 로컬 스택은 Docker Compose로 실행할 수 있습니다.

```bash
cp .env.example .env
# .env의 JWT_SECRET을 반드시 변경
docker compose up --build
curl http://localhost:8080/health
```

`COOKIE_SECURE=true`가 기본값입니다. 로컬 HTTP 확인에서만 `false`를 사용하고 HTTPS 운영 환경에서는 반드시 `true`를 유지하세요.

## 환경변수

| 이름 | 필수 | 기본값 | 설명 |
| --- | --- | --- | --- |
| `MYSQL_DATABASE` | 예 | 없음 | MySQL 데이터베이스 |
| `MYSQL_USER` | 예 | 없음 | MySQL 사용자 |
| `MYSQL_PASSWORD` | 예 | 없음 | MySQL 비밀번호 |
| `JWT_SECRET` | 예 | 없음 | 32바이트 이상 JWT 서명 키 |
| `MYSQL_HOST` | 아니요 | `127.0.0.1` | MySQL 호스트 |
| `MYSQL_PORT` | 아니요 | `3306` | MySQL 포트 |
| `REDIS_ADDR` | 아니요 | `127.0.0.1:6379` | Redis 주소 |
| `REDIS_PASSWORD` | 아니요 | 빈 값 | Redis 비밀번호 |
| `REDIS_DB` | 아니요 | `0` | Redis DB 번호 |
| `REDIS_TLS` | 아니요 | `false` | Redis TLS 사용 여부 |
| `IMAGE_UPLOAD_DIR` | 아니요 | `./images` | 이미지 저장 경로 |
| `ALLOWED_ORIGINS` | 아니요 | `http://localhost:5173` | 쉼표로 구분한 CORS origin |
| `COOKIE_SECURE` | 아니요 | `true` | Secure 쿠키 사용 여부 |
| `SPINE_PORT` | 아니요 | `8080` | HTTP 포트 |

비밀값은 소스나 이미지에 포함하지 않습니다. 원본 Spring 개발 설정의 평문 DB/JWT 값도 새 애플리케이션으로 복사하지 않았습니다.

## 기존 Spring 데이터로 전환

기존 JPA 테이블명과 컬럼명을 유지했으므로 새 애플리케이션의 MySQL 환경변수를 기존 `tech_blog` 데이터베이스로 지정할 수 있습니다. 실제 전환 전에는 반드시 데이터베이스와 이미지 디렉터리를 백업하고 staging 복제본에서 먼저 확인하세요.

`migrations/001_init.sql`은 새 환경용 스키마입니다. 기존 운영 데이터베이스에는 그대로 실행하지 말고 현재 인덱스·제약조건과 비교해 별도 변경 스크립트로 적용해야 합니다. Spring Data Redis의 기존 hash 직렬화와 새 문자열 토큰 형식은 호환되지 않으므로 Spine은 `spine:refreshToken:*` 별도 키를 사용하며 전환 시 모든 사용자가 다시 로그인해야 합니다. 기존 Redis 키 전체를 삭제하지 않습니다.

원본 `application.properties`에 노출된 DB/JWT 값은 이미 유출된 비밀로 간주해 운영 전 반드시 교체하세요.

## API

기본 경로는 `/api/v1`입니다.

| 영역 | 메서드 | 경로 | 인증 |
| --- | --- | --- | --- |
| Auth | POST | `/auth/login` | 공개 |
| Auth | POST | `/auth/logout` | 공개 |
| Users | POST | `/user/author-signup` | 관리자 |
| Users | POST | `/user/admin-signup` | 관리자 |
| Users | POST | `/user/normal-signup` | 공개 |
| Users | PATCH | `/user/{id}` | 필요 |
| Posts | POST | `/post?categoryId={id}` | 필요 |
| Posts | PATCH | `/post?postId={id}&categoryId={id}` | 필요 |
| Posts | GET | `/post?categoryName={name}&page=0&size=20` | 공개 |
| Posts | GET | `/post/slug/{slug}` | 공개 |
| Posts | GET | `/post/id/{id}` | 공개 |
| Posts | DELETE | `/post/{id}` | 필요 |
| Posts | POST | `/post/view-count/{id}` | 공개 |
| Posts | GET | `/post/search/recent?keyword={keyword}&page=0&size=20` | 공개 |
| Categories | POST | `/category?name={name}` | 필요 |
| Categories | GET | `/category/list` | 공개 |
| Likes | POST | `/like/{postId}` | 필요 |
| Likes | DELETE | `/like/{postId}` | 필요 |
| Likes | GET | `/like/{postId}` | 공개 |
| Images | POST | `/image` (`multipart/form-data`, `file`) | 공개, 원본 계약 유지 |
| Images | GET | `/image/{uuid}` | 공개 |

작성자와 관리자 계정은 기존 관리자만 생성할 수 있습니다. 일반 가입은 공개합니다. 글 작성은 작성자 또는 관리자, 수정과 삭제는 해당 글 작성자 또는 관리자, 카테고리 추가는 관리자만 가능합니다. 공개 작성자 정보에는 역할을 노출하지 않고, 프로필 수정 응답에는 자신의 역할을 포함합니다. 검색은 게시글 중복 없는 페이지 응답을 반환합니다.

## Spine CLI 계약 검증

```bash
spine inspect
spine contract check --json
spine contract diff
spine verify
```

`spine verify`는 정적 계약, 포맷, `go vet`, `go test`를 검증합니다. MySQL/Redis 연결, 실제 listener, 배포 환경 동작은 별도의 통합 검증 범위입니다.

## 구조

```text
internal/application/  # Spine composition root와 boot options
internal/blog/         # 유스케이스와 이미지 저장
internal/config/       # 환경변수 구성
internal/model/        # 도메인과 API DTO
internal/security/     # JWT, Redis 토큰 저장, 인증 인터셉터
internal/store/        # MySQL 저장소와 트랜잭션
src/*/                 # spine-cli가 만든 HTTP resource 모듈
migrations/            # MySQL 스키마
```

## GitHub Actions에서 OCI 배포

main push는 단위 테스트, race/vet/포맷 검사, 실제 MySQL 8.0 및 Redis 7 통합 테스트, Linux amd64 빌드 후 OCI 배포를 수행합니다. PR은 검증까지만 실행합니다. 운영 비밀값은 OCI의 root 전용 `/home/ubuntu/blog-spine/runtime.env`에 유지하고 GitHub에는 전용 SSH 키와 검증된 known_hosts만 보관합니다.

배포 스크립트는 기존 이미지 바인드 마운트를 유지하고 권한 enum에 ROLE_AUTHOR만 추가합니다. 새 포트를 검사한 다음 Nginx를 전환하며, 전환 실패 시 이전 설정으로 복구합니다. 성공 전에는 이전 컨테이너를 중지하지 않습니다. `/health` 외에 게시글 목록과 검색의 DB 조회도 검사합니다. 누락된 기존 이미지 31건의 메타데이터는 삭제하지 않습니다. 사용자가 확인한 원인은 과거 컨테이너 이미지 저장소의 마운트 누락입니다.

운영 상태: `/home/ubuntu/blog-spine/active-sha`, `active-port`. 릴리스: `/home/ubuntu/blog-spine/releases/<commit>`. 이전 Java 컨테이너 `blog-backend`와 원래 compose는 롤백용으로 보존합니다. 수동 롤백 시 Java 컨테이너를 시작하고 Nginx upstream을 8080으로 복원한 뒤 `nginx -t`와 reload를 실행합니다. DB를 과거 백업으로 덮어쓰지 않습니다.

최초 운영 전환에서는 DB 관리자 연결로 기존 enum의 순서를 유지한 채 ROLE_AUTHOR만 추가했습니다. 운영 앱 계정은 SELECT/INSERT/UPDATE/DELETE 권한을 유지합니다. 이후 배포의 migrate 명령은 이미 준비된 권한 스키마를 확인하며 재실행해도 데이터를 바꾸지 않습니다. 새 환경의 최초 enum 변경에 ALTER 권한이 없으면 DB 관리자 단계에서 먼저 적용해야 합니다.

Nginx 전환 직후 검증은 올바른 TLS 호스트명을 유지하면서 로컬 Nginx로 직접 연결합니다. 서버의 공인 IP로 되돌아가는 경로에 의존하지 않으며, 별도로 외부 클라이언트에서 공개 API를 확인합니다.

## 익명 댓글과 좋아요

- 댓글 조회/작성: `GET /api/v1/comments/post/{id}?page=0`, `POST /api/v1/comments/post/{id}`. 작성 본문은 nickname, content, password입니다.
- 댓글 수정/삭제: `PATCH`, `DELETE /api/v1/comments/{id}`. 비밀번호를 확인하며 관리자는 삭제할 수 있습니다. 비밀번호는 bcrypt 해시로만 저장하고 공개 응답에 포함하지 않습니다. 본문은 HTML이 아닌 일반 텍스트로 표시합니다.
- 좋아요 상태: `GET /api/v1/like/{id}/state`. 서명된 HttpOnly 방문자 쿠키를 발급하고 count, liked, anonymousCount, users를 반환합니다. 이후 `POST`, `DELETE /api/v1/like/{id}`는 로그인 없이 사용할 수 있습니다. 최초 쿠키 발급 전 쓰기는 거부합니다.
- DB의 게시글/방문자 및 게시글/사용자 고유 제약, 게시글 행 잠금으로 동시 중복 요청을 한 표로 처리합니다. 취소도 반복 호출할 수 있습니다. 기존 계정 좋아요를 이관하며 동일 브라우저의 익명 반응과 계정 반응은 합칩니다.
- 원본 IP는 저장하지 않습니다. Nginx가 덮어쓰는 실제 연결 IP를 HMAC 처리한 키로 분당 댓글 등록 5회, 수정/삭제 시도 10회, 좋아요 요청 60회를 제한합니다. Vercel 등 프록시 뒤의 연결 IP는 공유될 수 있습니다. 오래된 제한 기록은 정리합니다.
- 익명 중복 방지는 브라우저 식별을 기준으로 합니다. 쿠키 삭제나 다른 브라우저/IP를 이용하는 사람이 동일인인지 확정하는 기능은 아닙니다. 요청 제한은 초기화 후 빠른 반복을 줄이기 위한 추가 방어입니다.
- `migrations/002_feedback.sql`은 추가 테이블만 생성합니다. 운영 앱 계정의 권한을 늘리지 않고 최초 적용은 DB 관리자 연결에서 수행했습니다. 이후 migrate 명령은 기존 좋아요 이관과 집계 일치를 유지합니다.
- 로그인 여부에 따라 달라지는 댓글/반응 조회 응답은 no-store로 반환합니다. 이미지 저장소 바인드 마운트는 유지합니다.

## 관리자 전용 계측

`/admin/analytics`에서 1/7/30/90일 기준 조회, 익명 방문자, 세션, 평균 활성 체류시간, 스크롤, 유입 호스트, 접속 기기, 게시글별 조회를 확인합니다. 스낵바의 자기소개서/LinkedIn 클릭, 자기소개서 외부 링크, 500ms 이상 드래그한 8자 이상의 공개 문장을 집계하고 방문자별 이벤트를 페이지 단위로 조회합니다. 관리자 조회 API는 인증 후 DB의 ROLE_ADMIN을 다시 확인합니다. 클라이언트의 관리자 표시만으로 접근할 수 없습니다.

계측은 공개 페이지에서만 동작합니다. 로그인, 가입, 설정, 글쓰기 및 관리자 페이지는 제외하며 입력 내용이나 키 입력 자체를 수집하지 않습니다. 자기소개서 텍스트 선택은 최대 600자와 주변 문맥 1,000자를 전송합니다. 링크의 쿼리, fragment와 사용자 정보는 제거합니다. 원본 IP는 계측 DB에 보관하지 않고 Nginx가 제공하는 연결 IP의 HMAC으로 요청량을 제한합니다. 방문자는 별도 서명 HttpOnly 쿠키를 통해 익명으로 구분하며, 관리자 대시보드에는 HMAC 일부만 표시합니다. 실제 사람의 이름이나 신원은 알 수 없습니다.

탭이 보이고 포커스가 있으며 최근 1분 이내 활동이 있을 때 활성 시간을 누적합니다. 15초 heartbeat, 페이지 전환, visibilitychange, pagehide에서 전송하며 서버가 방문 경과시간을 상한으로 검증합니다. 종료 직전 네트워크 중단이나 차단 프로그램으로 일부 데이터가 누락될 수 있습니다. 방문/이벤트 ID를 통해 StrictMode와 재전송을 중복 집계하지 않습니다.

자기소개서의 공개 관심 수는 브라우저 식별 기준 누적 고유 방문자 수입니다. 관리자 자신과 기본적으로 식별 가능한 봇은 제외합니다. 상세 대시보드는 최근 90일까지 조회하며 오래된 상세 기록은 방문 요청 시 점진적으로 정리합니다. 누적 관심 수는 별도 테이블로 유지합니다. 계측 도입 이전의 체류시간과 클릭 이력은 복원하거나 만들어 넣지 않습니다. 게시글의 기존 누적 조회와 새로 측정한 기간별 조회는 별도 표시합니다.

`migrations/003_analytics.sql`은 계측용 테이블만 추가합니다. 운영 앱 계정 권한은 늘리지 않고 최초 준비는 DB 관리자 연결로 수행하며 이후 migrate 명령은 테이블 존재 여부를 확인합니다.

페이지 분석의 성능 영역은 서버 내부 처리(`Server-Timing: app`), 브라우저 API 왕복, 최초 콘텐츠 준비, SPA 전환 콘텐츠 준비, FCP/LCP/INP를 분리합니다. 관리자 전용 `GET /api/v1/admin/analytics/performance?days=30`은 기존과 같은 1/7/30/90일 범위에서 평균, p50, p95, p99, 최대, 표본 수와 HTTP 오류 수를 제공합니다. MySQL 8 윈도 함수로 전체 원시 표본의 nearest-rank 백분위를 계산하며, 페이지와 정규화한 API 경로별 상세도 제공합니다. 서버 지표는 실제 방문 브라우저가 받은 헤더 표본이며 전체 서버 요청 로그를 대신하지 않습니다. 캐시에서 재사용된 서버 헤더는 새 서버 표본으로 집계하지 않습니다. 백엔드 p95와 API p95의 차이는 개별 요청의 네트워크 시간이 아닙니다.

`migrations/004_performance.sql`을 DB 관리자 연결에서 먼저 적용한 뒤 백엔드와 프론트엔드를 배포해야 합니다. 기존 migrate 명령도 신규 테이블 준비와 존재 확인을 포함합니다. 기존 방문과 클릭 데이터는 유지하며 성능 표본은 적용 이후부터 수집합니다. 표본은 방문 기록에 연결되어 90일 보관 정책을 따르고, 동일 ID 재전송은 중복 집계하지 않으며 더 최신 revision만 반영합니다. 검색어·쿼리 문자열을 성능 API 경로에 저장하지 않습니다.
