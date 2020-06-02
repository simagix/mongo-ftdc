FROM golang:1.13-alpine as builder
RUN apk update && apk add dep git && rm -rf /var/cache/apk/* \
  && mkdir -p /go/src/github.com/simagix/mongo-ftdc
ADD . /go/src/github.com/simagix/mongo-ftdc
WORKDIR /go/src/github.com/simagix/mongo-ftdc
RUN dep ensure && go build -o simple_json simple_json.go
FROM alpine
LABEL Ken Chen <ken.chen@simagix.com>
RUN addgroup -S simagix && adduser -S simagix -G simagix
USER simagix
WORKDIR /home/simagix
COPY --from=builder /go/src/github.com/simagix/mongo-ftdc/simple_json /
WORKDIR /home/simagix
RUN mkdir diagnostic.data
CMD ["/simple_json", "--latest", "3", "diagnostic.data/"]