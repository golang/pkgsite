---
name: search-relevance-optimizer
description: Autonomous search relevance evaluation and RRF weight tuning agent. Use this skill to evaluate search quality, diagnose query failure modes, hypothesis-test RRF weights, and auto-refactor PostgreSQL scoring parameters.
---

# Autonomous Search Relevance Optimizer (ASRO) Skill

This skill equips the agent to act as an autonomous search quality engineer for `pkgsite`.

## Workflow & Reflection Loop

Follow this 5-step reflection loop when optimizing search relevance:

### 1. Step 1: Perceive (Run Evaluation Diagnostics)
Execute the `searcheval` tool to gather current baseline relevance metrics:
```bash
go run ./devtools/cmd/searcheval eval -vector-weight=1.0
```
Or run against production logs:
```bash
go run ./devtools/cmd/searcheval log-eval -log-queries=private/devtools/cmd/search/fresh_500.queries
```

### 2. Step 2: Reason & Diagnose Failure Modes
Inspect the per-query report table. Identify queries where expected packages receive **Rank > 3** or **0.000 MRR**:
* **Keyword Miss**: Keyword search missed the package completely due to strict term matching.
* **Vector Over-Match (Noise)**: Obscure third-party packages dominate the top 3 due to low popularity weighting.
* **RRF Dilution**: Reciprocal Rank Fusion weight for vector search is too low to elevate semantic hits.

Formulate an explicit hypothesis for weight adjustments:
> *"Hypothesis: Increasing VectorWeight to 2.0 will boost RRF score for Rank 2-4 semantic hits without degrading exact keyword matches."*

### 3. Step 3: Act (Execute Hypothesis Experiment)
Run `searcheval` with the proposed parameters:
```bash
go run ./devtools/cmd/searcheval eval -vector-weight=2.0 -popularity-weight=1.2
```

### 4. Step 4: Evaluate & Reflect
Compare the candidate MRR score against the baseline:
* If MRR increases: **Accept step** and log the improvement.
* If MRR decreases: **Reject step** and formulate a revised hypothesis (e.g. adjust `PopularityWeight`).

### 5. Step 5: Code Auto-Refactoring
Once peak MRR is reached, edit `internal/postgres/search.go` using `replace_file_content` to update `SearchScoringParams` with the optimal defaults.

```go
// In internal/postgres/search.go
var defaultScoringParams = SearchScoringParams{
    TextWeights:      [4]float64{0.1, 0.2, 1.0, 1.0},
    PopularityWeight: 1.0,
    VectorWeight:     2.0, // Updated by ASRO Agent Skill
}
```
