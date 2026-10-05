# 本番用イメージ。air を使わず、ビルド済みのバイナリだけを distroless で動かす。
# game / cms のテスト用環境（compose.test.yaml）でも使う
FROM golang:1.27.1-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG BUILD_VERSION=v0.0.0
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${BUILD_VERSION}" -o /app ./cmd/app

FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=build /app /app
EXPOSE 3333
USER nonroot:nonroot
ENTRYPOINT ["/app"]
