# syntax=docker/dockerfile:1
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/nebuladb ./cmd/nebuladb

FROM alpine:3.20
RUN adduser -D -u 65532 nebula
COPY --from=build /out/nebuladb /usr/local/bin/nebuladb
USER 65532:65532
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/usr/local/bin/nebuladb"]
CMD ["--data", "/data", "--http", ":8080", "--serve"]
