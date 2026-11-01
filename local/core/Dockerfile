FROM golang:1.27.3-bookworm

RUN go install github.com/air-verse/air@v1.67.4

# ホストのユーザー(uid 1000)と同じ uid で動かし、生成物（tmp/ 等）を root 所有にしない
ARG UID=1000
ARG GID=1000
RUN groupadd -g ${GID} dev && useradd -m -u ${UID} -g ${GID} dev
USER dev
WORKDIR /usr/src/app

# Go のモジュール・ビルドキャッシュを dev ユーザーが書ける場所に置く
ENV GOPATH=/home/dev/go
ENV GOCACHE=/home/dev/.cache/go-build

# ボリュームは初回作成時にマウント先の所有者を引き継ぐため、dev 所有で作っておく
RUN mkdir -p ${GOPATH} ${GOCACHE}
