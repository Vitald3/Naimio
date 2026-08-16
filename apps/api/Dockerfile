FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN test -f internal/auth/account.go && grep -q "func (h Handler) Me" internal/auth/account.go && grep -q "func (h Handler) Capability" internal/auth/account.go
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/dev-seed ./cmd/dev-seed

FROM alpine:3.20
RUN adduser -D -u 10001 app && mkdir -p /var/lib/naimio-media && chown -R app:app /var/lib/naimio-media
USER app
COPY --from=build /out/api /api
COPY --from=build /out/dev-seed /dev-seed
EXPOSE 8080
ENTRYPOINT ["/api"]
