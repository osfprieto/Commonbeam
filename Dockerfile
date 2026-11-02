FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /commonbeam ./cmd/commonbeam \
    && mkdir /state && chown 65532:65532 /state

FROM scratch
COPY --from=build /commonbeam /commonbeam
COPY --from=build --chown=65532:65532 /state /state
USER 65532:65532
WORKDIR /state
ENTRYPOINT ["/commonbeam"]
CMD ["help"]
