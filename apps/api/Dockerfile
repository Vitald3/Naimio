FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .


RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/api \
    ./cmd/api


RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/create-admin \
    ./cmd/create-admin



FROM alpine:3.20


RUN adduser -D -u 10001 app \
    && mkdir -p /var/lib/naimio-media \
    && chown -R app:app /var/lib/naimio-media


USER app


COPY --from=build /out/api /api
COPY --from=build /out/create-admin /create-admin


EXPOSE 8080


ENTRYPOINT ["/api"]
