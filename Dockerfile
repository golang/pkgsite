FROM golang:1.19 AS builder

WORKDIR /pkgsite-src
COPY . .

# Overrides for torq-version
RUN mv torq-overrides/footer.tmpl static/shared/footer/footer.tmpl
RUN mv torq-overrides/header.tmpl static/shared/header/header.tmpl
RUN mv torq-overrides/torq.svg static/shared/logo/torq.svg

RUN sed -i 's/pkg.go.dev/docs.torqio.dev/g' internal/frontend/fetch.go
RUN sed -i 's/pkg.go.dev/docs.torqio.dev/g' static/frontend/search/search.tmpl
RUN sed -i 's/go.dev/docs.torqio.dev/g' static/frontend/search/search.tmpl
RUN sed -i 's/go.dev/docs.torqio.dev/g' static/frontend/unit/_header.tmpl

# To allow symbols for our repos
RUN sed -i 's/return isRedistributable, append(lics, d.moduleLicenses...)/return true, append(lics, d.moduleLicenses...)/g' internal/licenses/licenses.go

RUN CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -o /pkgsite ./cmd/frontend/

FROM amd64/alpine
COPY --from=builder /pkgsite /
COPY --from=builder /pkgsite-src/static /static
COPY --from=builder /pkgsite-src/third_party /third_party

CMD ["/bin/sh", "-c", "/pkgsite -host 0.0.0.0:8080 --bypass_license_check -proxy_url $ATHNES_URL"]