FROM gliderlabs/alpine:3.1

RUN apk-install -t build-deps go git mercurial 

COPY ./src/main /go/src/github.com/christianwoehrle/docker-modjk-bridge
COPY VERSION  /go/src/github.com/christianwoehrle/docker-modjk-bridge/
RUN cd /go/src/github.com/christianwoehrle/docker-modjk-bridge \
    && export GOPATH=/go \
    && go get \
    && go build -ldflags "-X main.Version $(cat VERSION)" -o /bin/docker-modjk-bridge \
    && rm -rf /go \
    && apk del --purge build-deps

ENTRYPOINT ["/bin/docker-modjk-bridge"]

