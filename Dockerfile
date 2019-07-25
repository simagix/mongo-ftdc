FROM alpine
MAINTAINER Ken Chen <ken.chen@simagix.com>
ADD ftdc-linux-x64 /simple_json
RUN mkdir /diagnostic.data
CMD ["/simple_json", "/diagnostic.data"]
