module golang.org/x/discovery

go 1.12

require (
	github.com/google/go-cmp v0.2.1-0.20190217060313-77b690bf6c10
	github.com/lib/pq v1.0.0
	github.com/microcosm-cc/bluemonday v1.0.1
	gocloud.dev v0.12.0
	golang.org/x/net v0.0.0-20190213061140-3a22650c66bd
	golang.org/x/tools v0.0.0-20190226205152-f727befe758c
	google.golang.org/grpc v1.19.1
	gopkg.in/russross/blackfriday.v2 v2.0.1
	sos.googlesource.com/sos v1.0.0
)

replace sos.googlesource.com/sos v1.0.0 => ./sos.googlesource.com/sos

replace gopkg.in/russross/blackfriday.v2 v2.0.1 => github.com/russross/blackfriday/v2 v2.0.1
