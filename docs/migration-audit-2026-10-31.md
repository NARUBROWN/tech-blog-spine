> 아래 내용은 수정 전 조사 기록입니다. 후속 작업에서 권한 enum의 추가 마이그레이션, 프로필 role, 검색 페이지네이션과 서비스 권한 검사를 보완했습니다. 이미지 누락은 사용자가 과거 Docker 저장소 마운트 누락으로 확인했으며 기존 메타데이터는 유지합니다. 최신 배포 절차는 README와 deploy/deploy.sh를 따릅니다.

# Java 백엔드에서 Spine으로 전환하기 위한 점검

점검일: 2026-10-31. 운영 서버는 읽기 전용으로 점검했다. 서비스 교체, 데이터 변경, DB 마이그레이션, 이미지 업로드, 로그인은 수행하지 않았다.

## 빌드와 검증 결과

- Spine CLI 0.1.4로 schema, capabilities, context를 확인했다.
- `spine context --root /Users/kim-wonjeong/VscodeProjects/tech-blog-spine --json`: HTTP 경로 22개, 정적 분석 완료, 오류와 경고 없음.
- `spine build --root /Users/kim-wonjeong/VscodeProjects/tech-blog-spine`: macOS arm64 빌드 성공.
- `spine verify --root /Users/kim-wonjeong/VscodeProjects/tech-blog-spine --json`: 계약 검사, gofmt, vet, 테스트 통과.
- 임시 환경변수와 임시 이미지 디렉터리를 사용한 로컬 실행에서 `/health`가 HTTP 200과 `{"status":"ok"}`를 반환했다. 테스트 프로세스는 종료했다.
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 spine build --root /Users/kim-wonjeong/VscodeProjects/tech-blog-spine`: 성공. 현재 `bin/tech-blog-spine`은 운영 서버 아키텍처에 맞는 Linux x86-64 정적 실행 파일이다. macOS에서 다시 실행하려면 기본 빌드를 다시 해야 한다.
- `/health`는 DB와 Redis 연결을 확인하지 않는다. 빌드, 단위 테스트, 로컬 health 통과는 운영 데이터와 인증의 E2E 검증을 의미하지 않는다.
- 현재 소스 디렉터리에는 `.git`이 없다. 배포 전 원격 저장소 및 배포할 소스 리비전을 정하고 이미지 digest와 함께 기록해야 한다.

## 현재 운영 구조

- OCI `tech_blog` compartment, 춘천 리전.
- 앱: `superposition-app`, Ubuntu 22.04, Linux x86_64, VM.Standard.E2.1.Micro.
- 앱 컨테이너: `blog-backend`, `red3som/blog-backend:latest`, `/app/app.jar`, 호스트 8080 포트 사용.
- Nginx: `api.na2ru2.me`의 `/api/`를 `http://127.0.0.1:8080/api/`로 프록시한다.
- 이미지: `/home/ubuntu/blog/images`를 컨테이너 `/app/images`에 바인드 마운트한다.
- DB: 별도 `superposition-db` 인스턴스의 MySQL 8.0과 Redis 7. 앱은 내부 주소로 연결한다.
- 프런트엔드: 같은 출처의 `/api/v1` 호출을 Vercel rewrite로 API 서버에 전달한다. 전환 시 도메인과 API 경로를 유지하면 프런트엔드 baseURL 변경은 필요 없다.

## API 대조 결과와 전환 전 보완 사항

프런트엔드의 auth, category, image, post API 모듈이 호출하는 메서드와 경로는 Spine에 존재한다. 운영 게시글 목록의 응답 필드는 Spine의 PostResponse와 일치하며, 목록 페이지네이션과 카테고리 배열도 현재 프런트엔드에서 사용하는 형태를 제공한다. 다음 동작은 추가 보완 또는 검증이 필요하다.

1. **권한 모델과 운영 enum 불일치**
   - 운영 `user.role`은 `ROLE_ADMIN`, `ROLE_USER`, `ROLE_GUEST` enum이다. 현재 데이터는 관리자 2명, 일반 사용자 1명이다.
   - Spine은 `ROLE_AUTHOR`, `ROLE_ADMIN`, `ROLE_USER`를 정의하고 작성자 가입 시 `ROLE_AUTHOR`를 저장한다. 현재 DB에 그대로 작성하면 enum 제약에 걸린다.
   - 프런트엔드 글쓰기 메뉴 역시 `ROLE_AUTHOR` 또는 `ROLE_ADMIN`을 기대한다. 기존 ROLE_USER와 ROLE_GUEST의 의미를 원래 Java 정책과 대조한 뒤 명시적으로 매핑하고, enum 변경은 백업 및 별도 마이그레이션으로 적용해야 한다.
2. **프로필 수정 후 role 유실**
   - Spine `UpdateUser`는 role이 없는 UserResponse를 반환한다.
   - 프런트엔드 AuthContext는 수정 응답으로 현재 사용자 및 localStorage를 통째로 덮어쓴다. 저장 후 관리자/작성자 메뉴가 사라질 수 있다.
   - 로그인 사용자 응답 DTO와 공개 작성자 DTO를 분리하거나, 프런트엔드에서 기존 role을 유지하도록 정해야 한다.
3. **검색 결과 페이지네이션 미지원**
   - 프런트엔드는 keyword, page, size를 전달하고 totalPages를 받을 수 있도록 구현돼 있다.
   - Spine SearchRecent는 page를 읽지 않고 size 제한 배열을 반환한다. 결과가 많아도 다음 페이지를 표시할 수 없다.
   - 운영 Java 검색도 현재 배열을 반환하므로 새로 생긴 회귀라고 단정할 수는 없다. 최신 검색 화면의 페이지네이션 기능까지 완성하려면 서버의 COUNT/OFFSET와 PageResponse가 필요하다.
4. **인증과 게시글 권한 검사**
   - Spine 인터셉터는 비공개 요청의 로그인 여부를 검사하지만 게시글 수정 및 삭제 서비스에는 요청자 ID나 작성자/관리자 권한 검사가 전달되지 않는다. 카테고리 생성도 역할 검사가 없다.
   - 화면에서 버튼을 숨기는 것으로 API 권한이 보장되지 않는다. Java 정책과 대조하고 서버에서 작성자/관리자 권한을 적용한 뒤 인증 E2E를 확인해야 한다.
5. **기존 이미지 파일 누락**
   - 운영 image_metadata 41건의 stored_name과 이미지 디렉터리 전체 파일을 대조했다. 일치 10건, 파일이 없는 메타데이터 31건, 메타데이터 없는 파일 0건이다.
   - 이미지 누락은 점검 시점의 기존 상태이며 Spine으로 새로 유발된 문제가 아니다. 백업 복구 가능 여부 및 실제 게시글 참조 여부를 확인해야 한다.
   - Spine은 public_id로 메타데이터를 조회하고 stored_name으로 파일을 읽으므로 기존 UUID, 파일명, 경로를 유지해야 한다. 컨테이너 nonroot 사용자의 읽기 및 업로드 권한도 확인해야 한다.
6. **에디터 저장 데이터 검증**
   - 본문은 LONGTEXT이며 Spine은 HTML을 문자열로 저장한다. Quill/Mermaid 지원을 위해 새 API 경로를 추가할 필요는 없다.
   - 복제 DB에서 제목, 중첩 목록, 코드 블록, Mermaid data 속성을 포함한 본문의 저장과 재조회가 동일한지, 이미지 URL이 유지되는지 실제 왕복 검증이 필요하다.

## 전환 절차

1. API 호환성 보완과 권한 매핑을 먼저 확정하고 테스트한다. 기존 DB DDL과 필요한 변경만 비교한다. `migrations/001_init.sql`과 동봉 compose는 새 환경용이므로 운영 DB에 통째로 적용하지 않는다.
2. MySQL 일관성 백업과 이미지 디렉터리 백업을 생성하고 복원 가능 여부를 확인한다. 현재 이미지 누락 건은 별도 목록으로 관리한다. 검증용 복제 DB에서 필요한 스키마 변경을 연습한다.
3. Linux amd64 컨테이너 이미지를 고정 태그/digest로 빌드한다. 운영은 기존 MySQL 8.0, Redis 7과 이미지 바인드 마운트를 유지한다. 동봉 compose의 새 MySQL 8.4/빈 볼륨으로 바꾸지 않는다.
4. 기존 MYSQL_* 환경변수를 재사용하고, 기존 REDIS_HOST/REDIS_PORT를 Spine의 `REDIS_ADDR=호스트:포트`로 변환한다. `SPINE_PORT=8080`, `IMAGE_UPLOAD_DIR=/app/images`, `COOKIE_SECURE=true`, 실제 프런트엔드 도메인을 포함한 ALLOWED_ORIGINS를 명시한다. JWT 비밀값은 출력하거나 소스에 넣지 않는다.
5. 복제 DB와 별도 Redis DB/namespace를 사용해 새 컨테이너를 먼저 검증한다. 글 목록, slug/id 상세, 카테고리, 검색, 이미지의 읽기 비교를 수행하고, 쓰기와 로그인 테스트는 복제 환경에서 실행한다.
6. 앱 인스턴스가 Micro이므로 메모리 여유를 확인한 후 운영 DB 연결을 준비한다. Spine을 `127.0.0.1:18080 -> 8080` 등 별도 포트로 띄워 readiness와 주요 조회를 점검한다. health만으로 DB 연결 성공을 판정하지 않는다.
7. Java와 Go의 JWT/Redis 저장 방식이 다르므로 세션을 이어 쓸 것으로 가정하지 않는다. Redis 전체 FLUSH는 하지 않고 기존 로그인 키 범위를 확인해 새 namespace 또는 재로그인 정책을 적용한다.
8. 전환 직전 쓰기를 잠시 제한하고 최종 백업/검증 후 Nginx upstream만 새 포트로 바꾼다. `nginx -t` 성공 후 reload한다. 쿠키, CORS, 업로드 크기, 오류율을 확인한다.
9. 기존 Java 이미지와 compose는 롤백용으로 남긴다. 호환 가능한 스키마 변경을 사용하고 전환 후 쓰기 데이터를 보존한다. 문제가 생기면 Nginx를 기존 8080으로 되돌리되 DB와 이미지까지 과거 백업으로 무조건 덮어쓰지 않는다.

현재 결론: Spine 소스는 CLI 빌드와 기본 검증을 통과했고 운영 서버 접근도 가능하다. 운영 전환 전에는 권한 enum/정책, 프로필 응답, 검색 페이지네이션과 데이터 왕복 검증을 마쳐야 한다. 실제 운영 서비스는 아직 교체하지 않았다.
