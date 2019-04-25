FROM golang:1.12-alpine AS builder

LABEL maintainer="Julie Qiu <julie@golang.org>"

# Install the Certificate-Authority certificates for the app to be able to make
# calls to HTTPS endpoints.
RUN apk add --no-cache ca-certificates

ENV GOPROXY https://proxy.golang.org
ENV CGO_ENABLED 0
ENV GO111MODULE on

# Set the working directory outside $GOPATH to ensure module mode is enabled.
WORKDIR /src

COPY ./ ./

RUN go build -mod readonly -o /app ./cmd/fetch

RUN ls /usr/local/go
RUN ls /usr/local/go/bin

# Final stage: the running container.
FROM golang:1.12 AS final

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENV GOPATH /go
ENV PATH /usr/local/go/bin:$GOPATH/bin:$PATH

RUN go version
RUN which go

COPY --from=builder /app /app

EXPOSE 8080

ENTRYPOINT ["/app"]
