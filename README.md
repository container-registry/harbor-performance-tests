# Harbor Performance Tests

This repository contains a Go-native performance test framework for Harbor.

The current runner is `harborperf`, a single CLI that:

- prepares benchmark data on a target Harbor instance
- runs Harbor-specific benchmark scenarios
- writes structured JSON results
- generates markdown and HTML comparison reports

If you see Harbor performance issues, open an issue in [goharbor/harbor](https://github.com/goharbor/harbor).

## High-Level Architecture

`harborperf` is organized around Harbor workflows rather than generic HTTP load generation.

### Main components

- `cmd/harborperf`
  CLI entrypoint with `list`, `prepare`, `run`, `compare`, and `cleanup`
- `pkg/config`
  Environment-driven configuration, size presets, output paths, dataset policy
- `pkg/harbor`
  Native Harbor API and OCI push/pull client code
- `pkg/prepare`
  Dataset preparation pipeline: projects, users, members, artifacts, tags, audit logs, vulnerability prep
- `pkg/runner`
  Scenario lifecycle runner with setup, per-worker execution, teardown, and metrics collection
- `pkg/metrics`
  Latency summaries, success rate, throughput, summary JSON, and run metadata
- `pkg/report`
  Markdown report generation and HTML comparison charts
- `scenarios`
  Built-in Harbor benchmark scenarios
- `xk6-harbor`
  In-repo source of generated Harbor API types and client code reused by the native runner

### Execution model

Each scenario follows the same lifecycle:

1. Scenario setup
2. Worker initialization
3. Shared-iterations execution with configured workers
4. Scenario teardown
5. Summary and detailed result output

The current runner uses a closed workload model driven by:

- `HARBOR_VUS`
- `HARBOR_ITERATIONS`

### Dataset model

Benchmark data is managed by the prepare pipeline and controlled by:

- `HARBOR_SIZE=ci|small|medium`
- `HARBOR_DATASET_POLICY=fresh|verify|reuse`

Every run writes `dataset.json` with the resolved dataset contract and fingerprint. Comparisons use that fingerprint to prevent invalid A/B comparisons.

### Result artifacts

Each run writes artifacts into `./outputs` by default:

- `dataset.json`
- `<scenario>.summary.json`
- `<scenario>.run.json`
- `report.md` when `HARBOR_REPORT=true`
- `api-comparison.html` and `pull-push-comparison.html` for comparisons

## Prerequisites

- Go toolchain
- A reachable Harbor instance

Optional:

- Docker or Kubernetes only if you are provisioning Harbor locally yourself

## Build

Run directly:

```bash
go run ./cmd/harborperf list
```

Or build a binary:

```bash
go build -o harborperf ./cmd/harborperf
./harborperf list
```

## Configuration

The runner is configured through environment variables.

### Required

| Variable | Description |
|---|---|
| `HARBOR_URL` | Harbor URL in the form `http(s)://username:password@host` |

### Common

| Variable | Description | Default |
|---|---|---|
| `HARBOR_SIZE` | Dataset size preset: `ci`, `small`, `medium` | `small` |
| `HARBOR_VUS` | Worker count for scenario execution | preset-dependent |
| `HARBOR_ITERATIONS` | Total iterations shared across all workers | `2 x HARBOR_VUS` |
| `HARBOR_DATASET_POLICY` | `fresh`, `verify`, or `reuse` | `reuse` |
| `HARBOR_REPORT` | Generate `report.md` after a run | `false` |
| `HARBOR_OUTPUT_DIR` | Output directory for run artifacts | `./outputs` |
| `PROJECT_PREFIX` | Prefix for benchmark projects | `project` |
| `USER_PREFIX` | Prefix for benchmark users | `user` |
| `SCANNER_URL` | Scanner endpoint for vulnerability prep | unset |
| `FAKE_SCANNER_URL` | Fake scanner endpoint used during project prep | unset |
| `AUTO_SBOM_GENERATION` | Enable automatic SBOM generation on prepared projects | `false` |
| `BLOB_SIZE` | Blob size used for artifact generation | preset-dependent |
| `BLOBS_COUNT_PER_ARTIFACT` | Number of blobs per artifact | preset-dependent |

Compatibility aliases still accepted by the current runner:

- `K6_CSV_OUTPUT`
- `K6_JSON_OUTPUT`

## Commands

### List available scenarios

```bash
go run ./cmd/harborperf list
```

### Prepare benchmark data

Reuse existing benchmark-owned data when present:

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
HARBOR_DATASET_POLICY=reuse \
go run ./cmd/harborperf prepare
```

Recreate benchmark data from scratch:

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
HARBOR_DATASET_POLICY=fresh \
go run ./cmd/harborperf prepare
```

Verify the expected dataset exists without creating anything:

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
HARBOR_DATASET_POLICY=verify \
go run ./cmd/harborperf prepare
```

### Run all scenarios

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
HARBOR_VUS=10 \
HARBOR_ITERATIONS=10 \
HARBOR_REPORT=true \
go run ./cmd/harborperf run
```

### Run only API scenarios

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
HARBOR_VUS=10 \
HARBOR_ITERATIONS=10 \
go run ./cmd/harborperf run --api-only
```

### Run specific scenarios

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
HARBOR_VUS=20 \
HARBOR_ITERATIONS=100 \
go run ./cmd/harborperf run list-projects get-project
```

### Cleanup benchmark-owned data

```bash
HARBOR_URL=http://admin:Harbor12345@localhost:8080 \
HARBOR_SIZE=ci \
go run ./cmd/harborperf cleanup
```

### Compare two result directories

```bash
go run ./cmd/harborperf compare ./results/run-a ./results/run-b
```

The compare command expects both directories to contain:

- `dataset.json`
- `*.summary.json`

It refuses to compare runs when dataset fingerprints do not match.

## Current Built-In Scenarios

Use `harborperf list` for the source of truth. The built-in set currently includes:

- `get-artifact-by-digest`
- `get-artifact-by-tag`
- `get-catalog`
- `get-project`
- `get-repository`
- `get-v2`
- `list-artifact-tags`
- `list-artifacts`
- `list-audit-logs`
- `list-project-logs`
- `list-project-members`
- `list-projects`
- `list-quotas`
- `list-repositories`
- `list-users`
- `pull-artifacts-from-different-projects`
- `pull-artifacts-from-same-project`
- `push-artifacts-to-different-projects`
- `push-artifacts-to-same-project`
- `search-users`

## CI/CD Usage

The same commands used locally are intended to run in CI:

1. `harborperf prepare`
2. `harborperf run`
3. archive the output directory
4. optionally run `harborperf compare` against a stored baseline

Recommended CI environment for a short validation run:

```bash
HARBOR_SIZE=ci
HARBOR_VUS=10
HARBOR_ITERATIONS=10
HARBOR_DATASET_POLICY=fresh
HARBOR_REPORT=true
```

## Example Local Workflow

Against a local Harbor on `localhost:8080`:

```bash
export HARBOR_URL=http://admin:Harbor12345@localhost:8080
export HARBOR_SIZE=ci
export HARBOR_VUS=10
export HARBOR_ITERATIONS=10
export HARBOR_REPORT=true

go run ./cmd/harborperf prepare
go run ./cmd/harborperf run --api-only
```

Then inspect the outputs:

```bash
ls ./outputs
cat ./outputs/dataset.json
cat ./outputs/report.md
```

## Notes

- `harborperf list` does not require Harbor connectivity.
- `prepare` and `cleanup` only target benchmark-owned resources based on the configured naming prefixes.
- The legacy `mage` and `scripts/` flow is still present in the repository, but the native Go CLI is the current primary path.
