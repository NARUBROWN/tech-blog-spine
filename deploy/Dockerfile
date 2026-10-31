FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 app && adduser -D -u 10001 -G app app
WORKDIR /app
COPY tech-blog-spine /usr/local/bin/tech-blog-spine
COPY migrate /usr/local/bin/migrate
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tech-blog-spine"]
