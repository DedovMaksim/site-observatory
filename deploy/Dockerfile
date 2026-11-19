FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN mkdir -p /out/data && chown 65532:65532 /out/data
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/site-observatory ./cmd/server
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/site-observatory /app/site-observatory
COPY --from=build --chown=65532:65532 /out/data /data
ENV APP_ADDR=0.0.0.0:8080 DATA_DIR=/data
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/app/site-observatory"]
