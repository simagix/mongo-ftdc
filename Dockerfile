FROM golang:alpine
MAINTAINER Ken Chen <ken.chen@simagix.com>
RUN apk update && apk add dep && rm -rf /var/cache/apk/* \
  && mkdir -p /go/src/github.com/simagix/mongo-ftdc
ADD . /go/src/github.com/simagix/mongo-ftdc
WORKDIR /go/src/github.com/simagix/mongo-ftdc
RUN dep ensure && go build -o /simple_json simple_json.go && rm -rf /go/src/*
RUN addgroup -S simagix && adduser -S simagix -G simagix
USER simagix
WORKDIR /home/simagix
RUN mkdir diagnostic.data
CMD ["/simple_json", "diagnostic.data/"]
