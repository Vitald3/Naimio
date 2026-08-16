FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/worker ./cmd/worker

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/worker /worker
EXPOSE 8081
ENTRYPOINT ["/worker"]
