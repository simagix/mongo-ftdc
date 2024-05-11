FROM golang:1.19-alpine as builder
RUN apk update && apk add git bash && rm -rf /var/cache/apk/*
# ADD . /github.com/simagix/mongo-ftdc
WORKDIR /mongo-ftdc
COPY . /mongo-ftdc/
RUN ./build.sh
FROM alpine
LABEL Ken Chen <ken.chen@simagix.com>
RUN addgroup -S ajithkn && adduser -S ajithkn -G ajithkn
USER ajithkn
WORKDIR /home/ajithkn
COPY --from=builder /mongo-ftdc/dist/ftdc_json /ftdc_json
CMD ["/ftdc_json", "--latest", "3", "diagnostic.data/"]
