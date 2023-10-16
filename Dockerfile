FROM golang:1.19-alpine as builder
RUN apk update && apk add git bash && rm -rf /var/cache/apk/* \
  && mkdir -p /github.com/simagix/mongo-ftdc
ADD . /github.com/simagix/mongo-ftdc
WORKDIR /github.com/simagix/mongo-ftdc
RUN ./build.sh
FROM alpine
LABEL Ken Chen <ken.chen@simagix.com>
RUN addgroup -S simagix && adduser -S simagix -G simagix
USER simagix
WORKDIR /home/simagix
COPY --from=builder /github.com/simagix/mongo-ftdc/dist/ftdc_json /ftdc_json
CMD ["/ftdc_json", "--latest", "3", "diagnostic.data/"]
