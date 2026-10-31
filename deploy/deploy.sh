#!/usr/bin/env bash
# GitHub Actions가 전달한 검증 완료 바이너리를 OCI에서 교체합니다.
set -Eeuo pipefail
release=$(cd -- "$(dirname -- "$0")" && pwd)
sha=${1:?커밋 SHA가 필요합니다}
[[ $sha =~ ^[0-9a-f]{40}$ ]] || exit 2
root=/home/ubuntu/blog-spine
mkdir -p "$root"
exec 9>"$root/deploy.lock"
flock -w 600 9
cd "$release"
sha256sum -c SHA256SUMS
image="tech-blog-spine:$sha"
docker build -f Dockerfile -t "$image" .
# 비밀값은 서버 안에서만 읽고 root 소유의 파일에 보관합니다.
if [[ ! -f "$root/runtime.env" ]]; then
 python3 - "$root/runtime.env" <<'PY'
import json,subprocess,sys,secrets,os
j=json.loads(subprocess.check_output(['docker','inspect','blog-backend']))[0]
e=dict(x.split('=',1) for x in j['Config']['Env'] if '=' in x)
v={k:e[k] for k in ['MYSQL_HOST','MYSQL_PORT','MYSQL_DATABASE','MYSQL_USER','MYSQL_PASSWORD']}
v.update(REDIS_ADDR=e.get('REDIS_HOST','127.0.0.1')+':'+e.get('REDIS_PORT','6379'),JWT_SECRET=secrets.token_urlsafe(48),IMAGE_UPLOAD_DIR='/app/images',SPINE_PORT='8080',COOKIE_SECURE='true',ALLOWED_ORIGINS='https://www.na2ru2.me,https://na2ru2.me,http://localhost:5173')
for k in ['REDIS_PASSWORD']:
 if e.get(k):v[k]=e[k]
fd=os.open(sys.argv[1],os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'w') as f:
 for k,x in v.items():
  if '\n' in x or '\r' in x:raise ValueError('환경변수에 개행이 있습니다')
  f.write(k+'='+x+'\n')
PY
fi
# 이미지는 영속 호스트 디렉터리를 유지하고, 비루트 컨테이너에 접근권만 부여합니다.
image_dir=/home/ubuntu/blog/images
if ! command -v setfacl >/dev/null; then echo 'setfacl이 필요합니다' >&2; exit 1; fi
setfacl -m u:10001:rwx "$image_dir"
setfacl -d -m u:10001:rwx "$image_dir"
find "$image_dir" -type f -exec setfacl -m u:10001:rw {} +
docker run --rm --env-file "$root/runtime.env" --entrypoint /usr/local/bin/migrate "$image"
old_port=$(cat "$root/active-port" 2>/dev/null || echo 8080)
port=18080
[[ $old_port == 18080 ]] && port=18081
name="blog-spine-$port"
docker rm -f "$name" >/dev/null 2>&1 || true
docker run -d --name "$name" --restart unless-stopped --env-file "$root/runtime.env" -p "127.0.0.1:$port:8080" -v "$image_dir:/app/images" "$image" >/dev/null
nginx_file=$(grep -Rl 'proxy_pass http://127.0.0.1:' /etc/nginx/sites-enabled | head -1)
[[ -n $nginx_file ]] || { docker rm -f "$name"; exit 1; }
backup="$root/nginx-before-$sha.conf"
cp -L "$nginx_file" "$backup"
rollback(){
 cp "$backup" "$nginx_file"
 nginx -t && systemctl reload nginx
 docker rm -f "$name" >/dev/null 2>&1 || true
 echo '배포 실패: 이전 Nginx 설정으로 복구했습니다' >&2
}
trap rollback ERR
for attempt in $(seq 1 30); do
 if curl -fsS "http://127.0.0.1:$port/health" >/dev/null && curl -fsS "http://127.0.0.1:$port/api/v1/post?page=0&size=1" | python3 -c 'import json,sys;assert isinstance(json.load(sys.stdin)["content"],list)'; then break; fi
 [[ $attempt -lt 30 ]] || false
 sleep 2
done
# 실제 이미지 저장소도 읽기 검증합니다. 누락된 기존 파일은 새로 삭제하지 않습니다.
python3 - "$port" <<'PY'
import json,urllib.request,sys
base='http://127.0.0.1:'+sys.argv[1]
for path in ['/api/v1/category/list','/api/v1/post/search/recent?keyword=LLM&page=0&size=1']:
 with urllib.request.urlopen(base+path,timeout=10) as r:json.load(r)
PY
sed -i "s|127.0.0.1:$old_port/api/|127.0.0.1:$port/api/|g" "$nginx_file"
if ! grep -q 'X-Feedback-IP' "$nginx_file"; then
 sed -i '/proxy_pass http:\/\/127.0.0.1:/a\        proxy_set_header X-Feedback-IP $remote_addr;' "$nginx_file"
fi
nginx -t
systemctl reload nginx
verified=false
for attempt in $(seq 1 10); do
 if curl -fsS --retry 3 --retry-all-errors --retry-delay 1 --max-time 10 --resolve api.na2ru2.me:443:127.0.0.1 'https://api.na2ru2.me/api/v1/post/search/recent?keyword=LLM&page=0&size=1' | python3 -c 'import json,sys;d=json.load(sys.stdin);assert isinstance(d,dict) and isinstance(d["content"],list) and "totalPages" in d'; then
  verified=true
  break
 fi
 sleep 1
done
[[ $verified == true ]]
printf '%s\n' "$port" > "$root/active-port"
printf '%s\n' "$sha" > "$root/active-sha"
trap - ERR
if [[ $old_port == 8080 ]]; then docker stop blog-backend >/dev/null; else docker stop "blog-spine-$old_port" >/dev/null; fi
echo "Spine 배포 완료: $sha (포트 $port)"
