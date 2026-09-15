// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"go.opencensus.io/plugin/ochttp"
	"golang.org/x/pkgsite/internal/config"
	"golang.org/x/pkgsite/internal/postgres"
	"golang.org/x/pkgsite/internal/proxy"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/worker"
)

var defaultModulesToSeed = []string{
	"github.com/go-chi/chi/v5@v5.0.12",
	"github.com/gorilla/mux@v1.8.1",
	"github.com/julienschmidt/httprouter@v1.3.0",
	"github.com/gin-gonic/gin@v1.9.1",
	"github.com/labstack/echo/v4@v4.11.4",
	"github.com/gofiber/fiber/v2@v2.52.2",
	"github.com/valyala/fasthttp@v1.52.0",
	"github.com/beego/beego/v2@v2.1.6",
	"github.com/go-resty/resty/v2@v2.11.0",
	"github.com/gorilla/websocket@v1.5.1",
	"github.com/lesismal/nbio@v1.5.2",
	"github.com/json-iterator/go@v1.1.12",
	"github.com/goccy/go-json@v0.10.2",
	"github.com/bytedance/sonic@v1.11.3",
	"github.com/tidwall/gjson@v1.17.1",
	"github.com/tidwall/sjson@v1.2.5",
	"github.com/buger/jsonparser@v1.1.1",
	"go.uber.org/zap@v1.27.0",
	"github.com/rs/zerolog@v1.32.0",
	"github.com/sirupsen/logrus@v1.9.3",
	"github.com/apex/log@v1.9.0",
	"github.com/op/go-logging@v0.0.0-20160315200105-970db520ece7",
	"github.com/lib/pq@v1.10.9",
	"github.com/jackc/pgx/v5@v5.5.5",
	"github.com/go-sql-driver/mysql@v1.8.0",
	"github.com/mattn/go-sqlite3@v1.14.22",
	"modernc.org/sqlite@v1.29.5",
	"gorm.io/gorm@v1.25.7",
	"github.com/jmoiron/sqlx@v1.3.5",
	"github.com/uptrace/bun@v1.1.17",
	"entgo.io/ent@v0.13.1",
	"github.com/redis/go-redis/v9@v9.5.1",
	"github.com/gomodule/redigo@v1.9.2",
	"github.com/dgraph-io/badger/v4@v4.2.0",
	"go.etcd.io/bbolt@v1.3.9",
	"github.com/syndtr/goleveldb@v1.0.0",
	"github.com/spf13/cobra@v1.8.0",
	"github.com/spf13/viper@v1.18.2",
	"github.com/urfave/cli/v2@v2.27.1",
	"github.com/alecthomas/kong@v0.9.0",
	"github.com/knadh/koanf/v2@v2.1.0",
	"github.com/joho/godotenv@v1.5.1",
	"github.com/kelseyhightower/envconfig@v1.4.0",
	"github.com/google/uuid@v1.6.0",
	"github.com/oklog/ulid/v2@v2.1.0",
	"github.com/segmentio/ksuid@v1.0.4",
	"github.com/stretchr/testify@v1.9.0",
	"github.com/onsi/ginkgo/v2@v2.16.0",
	"github.com/onsi/gomega@v1.31.1",
	"github.com/golang/mock@v1.6.0",
	"go.uber.org/mock@v0.4.0",
	"github.com/google/go-cmp@v0.6.0",
	"google.golang.org/grpc@v1.62.1",
	"google.golang.org/protobuf@v1.33.0",
	"github.com/golang/protobuf@v1.5.4",
	"github.com/golang-jwt/jwt/v5@v5.2.1",
	"github.com/dgrijalva/jwt-go@v3.2.0+incompatible",
	"golang.org/x/crypto@v0.21.0",
	"golang.org/x/sync@v0.6.0",
	"golang.org/x/net@v0.22.0",
	"golang.org/x/sys@v0.18.0",
	"golang.org/x/text@v0.14.0",
	"golang.org/x/time@v0.5.0",
	"golang.org/x/oauth2@v0.18.0",
	"cloud.google.com/go/storage@v1.39.1",
	"cloud.google.com/go/pubsub@v1.36.2",
	"github.com/aws/aws-sdk-go-v2@v1.26.0",
	"github.com/aws/aws-sdk-go@v1.51.6",
	"github.com/prometheus/client_golang@v1.19.0",
	"go.opentelemetry.io/otel@v1.24.0",
	"go.opentelemetry.io/otel/trace@v1.24.0",
	"github.com/nats-io/nats.go@v1.33.1",
	"github.com/rabbitmq/amqp091-go@v1.9.0",
	"github.com/IBM/sarama@v1.43.0",
	"github.com/segmentio/kafka-go@v0.4.47",
	"github.com/streadway/amqp@v1.1.0",
	"github.com/vmihailenco/msgpack/v5@v5.4.1",
	"gopkg.in/yaml.v3@v3.0.1",
	"gopkg.in/yaml.v2@v2.4.0",
	"github.com/pelletier/go-toml/v2@v2.1.1",
	"github.com/mitchellh/mapstructure@v1.5.0",
	"github.com/pkg/errors@v0.9.1",
	"github.com/uber/jaeger-client-go@v2.30.0+incompatible",
	"github.com/graphql-go/graphql@v0.8.1",
	"github.com/99designs/gqlgen@v0.17.45",
	"github.com/hashicorp/golang-lru/v2@v2.0.7",
	"github.com/patrickmn/go-cache@v2.1.0+incompatible",
	"github.com/coocood/freecache@v1.2.4",
	"github.com/allegro/bigcache/v3@v3.1.0",
	"github.com/robfig/cron/v3@v3.0.1",
	"github.com/go-co-op/gocron@v1.37.0",
	"github.com/panjf2000/ants/v2@v2.9.0",
	"github.com/gammazero/workerpool@v1.1.3",
	"github.com/sourcegraph/conc@v0.3.0",
	"github.com/charmbracelet/bubbletea@v0.25.0",
	"github.com/charmbracelet/lipgloss@v0.10.0",
	"github.com/fatih/color@v1.16.0",
	"github.com/schollz/progressbar/v3@v3.14.2",
	"github.com/olekukonko/tablewriter@v0.0.5",
}

func runSeed(ctx context.Context, db *postgres.DB, cfg *config.Config, customModules []string) {
	if cfg.DBPort != "5432" && os.Getenv("GO_DISCOVERY_ALLOW_SEED") != "true" {
		log.Fatalf("ABORTED: Attempted to run seed against non-standard database (port %s). Seeding modifies database records and should NOT be run on Cloud SQL instances. Set GO_DISCOVERY_ALLOW_SEED=true to override.", cfg.DBPort)
	}

	modulesToSeed := defaultModulesToSeed
	if len(customModules) > 0 {
		modulesToSeed = customModules
	}

	proxyClient, err := proxy.New(cfg.ProxyURL, new(ochttp.Transport))
	if err != nil {
		log.Fatalf("proxy.New: %v", err)
	}

	sourceClient := source.NewClient(&http.Client{
		Transport: new(ochttp.Transport),
		Timeout:   config.SourceTimeout,
	})

	f := &worker.Fetcher{
		ProxyClient:  proxyClient,
		SourceClient: sourceClient,
		DB:           db,
	}

	log.Printf("Seeding %d modules into test database...", len(modulesToSeed))
	for _, m := range modulesToSeed {
		parts := strings.Split(m, "@")
		path, ver := parts[0], parts[1]

		log.Printf("Fetching %s@%s...", path, ver)
		if _, _, err := f.FetchAndUpdateState(ctx, path, ver, ""); err != nil {
			log.Printf("Failed to seed module %s@%s: %v", path, ver, err)
		}
	}
	log.Println("Seeding complete.")
}
