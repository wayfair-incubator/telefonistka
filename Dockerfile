
FROM golang:1.21.1 as test
ARG GOPROXY
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
WORKDIR /go/src/github.com/wayfair-incubator/telefonistka
COPY . ./
RUN make test

FROM test as build
# FROM golang:1.18.3 as build
ARG GOPROXY
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
WORKDIR /go/src/github.com/wayfair-incubator/telefonistka
COPY . ./
RUN make build





FROM scratch
ENV wf_version="0.0.5"
ENV wf_description="K8s team GitOps prmoter webhook server"
WORKDIR /telefonistka
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /go/src/github.com/wayfair-incubator/telefonistka/telefonistka /telefonistka/bin/telefonistka
COPY templates/ /telefonistka/templates/
# This next line is hack to overcome GH actions lack of support for docker workdir override https://github.com/actions/runner/issues/878
COPY templates/ /github/workspace/templates/
USER 1001
ENTRYPOINT ["/telefonistka/bin/telefonistka"]
CMD ["server"]

