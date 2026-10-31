FROM golang:1.25.5-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tech-blog-spine .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/tech-blog-spine /usr/local/bin/tech-blog-spine
RUN mkdir -p /app/images && chown -R app:app /app
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tech-blog-spine"]
