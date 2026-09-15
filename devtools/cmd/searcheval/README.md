# Search Quality Evaluator (`searcheval`)

`searcheval` is a command-line tool for benchmarking, evaluating, and tuning search relevance for `pkgsite`. It supports comparing standard PostgreSQL full-text keyword search (`ts_rank`) against **Hybrid Vector Search** combining keyword scoring with Vertex AI text embeddings via **Reciprocal Rank Fusion (RRF)**.

---

## Key Concepts & Evaluation Metrics

* **[Reciprocal Rank Fusion (RRF)](https://en.wikipedia.org/wiki/Reciprocal_rank_fusion)**: This is an informatioan retrieval aggregation technique that enables us to combine rankings from distinct search algorithms - in our system this is text search and vector search. For example, text search outputs scores with arbitrary scales (e.g. 0.15, 0.24) but vector search outputs similarity on a scale from (-1.0 and 1.0). RRF ignores the raw score, but just looks at the rank order. See [Cormack et al. (2009)](https://cormack.uwaterloo.ca/cormacksigir09-rrf.pdf).
* **[Mean Reciprocal Rank (MRR)](https://en.wikipedia.org/wiki/Mean_reciprocal_rank)**: A statistic evaluating ranking quality against target ground-truth packages. For a single query, the Reciprocal Rank (RR) is $\frac{1}{\text{rank}}$ of the first expected target package (or $0.0$ if outside top-$N$). **MRR@5** is the average of Reciprocal Ranks across all golden queries.

---

## Prerequisites & Database Setup

### 1. Start Cloud SQL Proxy

Choose one of the following options to launch the database proxy on port `5431`:

* **Option A: Connect to Main Dev Database (`dev-discovery-db`)**
  ```bash
  ./private/devtools/run_cloud_sql_proxy.sh dev
  ```

* **Option B: Connect to Custom / Cloned Database Instance (`dev-discovery-db-clone`)**
  ```bash
  cloud-sql-proxy --port 5431 go-discovery:us-central1:dev-discovery-db-clone
  ```

### 2. Export Required Environment Variables

Set your GCP project, Cloud SQL password, and connection parameters:

```bash
export GOOGLE_CLOUD_PROJECT=go-discovery
export GO_DISCOVERY_DATABASE_PASSWORD=<password>
export GO_DISCOVERY_DATABASE_PORT=5431
export GO_DISCOVERY_DATABASE_NAME=dev-discovery-db
```

---

### 1. Golden Query Evaluation (`eval`)

Runs search quality benchmarks against a curated set of ground-truth queries ([`golden_queries.json`](testdata/golden_queries.json)) and reports **Mean Reciprocal Rank (MRR@5)** metrics. Each cell displays the target package's **1-based Rank** and its corresponding **Reciprocal Rank score** (`1/rank`), which is averaged in the summary row.

```bash
go run ./devtools/cmd/searcheval eval
```

**Output Example:**

```
Query                     | Target Package                      | Keyword     | Vector      | Hybrid
----------------------------------------------------------------------------------------------------
http router               | github.com/go-chi/chi/v5            | #2 (0.500)  | #6 (0.167)  | #2 (0.500)
structured logger         | go.uber.org/zap                     | >5 (0.000)  | #4 (0.250)  | >5 (0.000)
web framework             | github.com/gin-gonic/gin            | #1 (1.000)  | #3 (0.333)  | #1 (1.000)
command line parser       | github.com/spf13/cobra              | >5 (0.000)  | #2 (0.500)  | #4 (0.250)
orm                       | gorm.io/gorm                        | #2 (0.500)  | #1 (1.000)  | #1 (1.000)
----------------------------------------------------------------------------------------------------
MEAN RECIPROCAL RANK (MRR) |                                     | 0.479       | 0.449       | 0.562
```

---

### 2. Bulk Production Query Log Evaluation (`log-eval`)

Evaluates performance and zero-result recovery rates across unlabelled, real-world production query logs (e.g. extracted via `queries_for_compare.sh` or log exports). This will enable one to benchmark against real user search traffic.

```bash
# Optional: Pull fresh query logs from Cloud Logging for the last 24h
./private/devtools/cmd/search/queries_for_compare.sh 24h > fresh_24h.queries

# Run comparative log evaluation
go run ./devtools/cmd/searcheval -log-queries=fresh_24h.queries log-eval
```

**Metrics Measured:**
* **Keyword Zero-Result Queries**: Count and percentage of production queries returning 0 results with standard keyword search.
* **Hybrid Zero-Result Queries**: Count and percentage of queries returning 0 results with hybrid search.
* **Zero-Result Recoveries**: Queries where keyword search returned 0 results, but hybrid search successfully surfaced $\ge 1$ result via vector search.
* **Average Result Overlap**: Mean top-5 result set overlap between standard keyword search and hybrid search across queries.

---

### 3. Interactive Query Debugger (`search`)

Runs a single search query string and displays top 10 search results side-by-side across Keyword, Pure Vector, and Hybrid RRF modes:

```bash
go run ./devtools/cmd/searcheval search "structured logger"
```

---

### 4. Embedding Backfill (`embed`)

Scans PostgreSQL for active packages (`imported_by_count >= 1 AND embedding IS NULL`), generates 256-dimensional text embeddings via Vertex AI (`text-embedding-004`), and updates `search_documents.embedding` in the connected database:

```bash
go run ./devtools/cmd/searcheval embed
```

---

### 5. Local Database Seeding (`seed`)

> [!CAUTION]
> Do **NOT** run `searcheval seed` while connected to Cloud SQL database proxies (`dev`, `staging`, `prod`). `seed` writes module data into PostgreSQL and is intended **ONLY** for fresh local Postgres instances (`localhost:5432`). `searcheval` automatically blocks `seed` when connected to non-standard ports unless `GO_DISCOVERY_ALLOW_SEED=true` is set.

Populates a fresh local PostgreSQL database with popular Go modules (e.g. `chi`, `gin`, `zap`, `pgx`, `cobra`, `viper`, `gorm`, `uuid`, `grpc`, `testify`, `zerolog`, `bun`, `badger`, `bbolt`, etc.) fetched directly from `proxy.golang.org`:

```bash
# Seed default set of 100 popular benchmark modules
go run ./devtools/cmd/searcheval seed

# Or seed specific custom modules
go run ./devtools/cmd/searcheval seed github.com/go-chi/chi/v5@v5.0.12 github.com/gin-gonic/gin@v1.9.1
```

---

## Search & Scoring Parameter Flags

You can experiment with search scoring parameters (such as feature weights and result limits) dynamically without recompiling code:

| Flag | Description | Default | Example |
| :--- | :--- | :--- | :--- |
| `-text-weights` | Custom `ts_rank` section weights `(D,C,B,A)` | DB default (`0.1,0.2,1.0,1.0`) | `-text-weights="0.1,0.5,1.0,2.0"` |
| `-popularity-weight` | Exponent for package import count popularity | DB default (`1.0`) | `-popularity-weight=0.8` |
| `-vector-weight` | RRF vector search weight in hybrid fusion | DB default (`1.0`) | `-vector-weight=1.5` |
| `-limit` | Number of search results to display per engine | `10` | `-limit=20` |
| `-queries` | Path to golden queries JSON file | `testdata/golden_queries.json` | `-queries=custom_queries.json` |

**Example using custom weights:**

```bash
go run ./devtools/cmd/searcheval -vector-weight=1.2 -popularity-weight=0.8 eval
```
