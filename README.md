# gale

Fast, resumable macOS package manager written in Go with a Bubble Tea terminal UI.

## Features

- **Native Go binary** — no Ruby runtime, minimal startup overhead
- **Resumable chunked downloads** — 8MB chunks, parallel workers, sidecar manifests survive crashes and disconnects
- **Content-addressed store** — deduplicated artifacts at `~/.gale/store/<sha256>/`
- **APFS clonefile / hardlink installs** — near-instant materialization into `~/.gale/opt`
- **Homebrew bottle bootstrap** — uses [formulae.brew.sh](https://formulae.brew.sh/docs/api/) JSON API as the v1 index source
- **Bubble Tea TUI** — install progress, search, dependency tree, package detail, installed list

## Installation

Requires **macOS** (Apple Silicon or Intel) and **Go 1.22+** to build, or install the prebuilt binary once the repo is published.

### Install from GitHub (recommended)

After pushing this repo to GitHub:

```bash
go install github.com/ganeshdipdumbare/gale/cmd/gale@latest
```

Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on your `PATH`:

```bash
export PATH="$HOME/go/bin:$HOME/.gale/bin:$PATH"
```

### Build from source

```bash
git clone https://github.com/ganeshdipdumbare/gale.git
cd gale
make build
sudo mv bin/gale /usr/local/bin/gale   # optional: install globally
```

### First run

```bash
gale update                              # fetch package index
gale install wget                        # install a package (+ deps)
gale list                                # show installed packages
```

Package binaries are linked into `~/.gale/bin` — add that directory to your `PATH` (see above).

Set `GALE_HOME` to override the data directory (default `~/.gale`).

## Quick start

For local development without installing globally:

```bash
make build
./bin/gale update
./bin/gale install wget
./bin/gale list
```

## Architecture

```
cmd/gale/           CLI (cobra)
internal/download/  Resumable chunked downloader + .gale-part.json sidecar
internal/index/     Homebrew API adapter, msgpack+zstd local cache
internal/resolver/  Dependency DAG, topo sort, parallel install
internal/store/     Content-addressed store, bottle extraction, clonefile
internal/db/        SQLite metadata (modernc.org/sqlite, pure Go)
internal/tui/       Bubble Tea models (progress, search, tree, detail, table)
```

### Resumable download sidecar

Partial downloads persist a `<file>.gale-part.json` manifest tracking:

- Total size, chunk size, per-chunk offset/size/completion
- Per-chunk SHA256 after verification
- Mirror list and source URL

On resume, existing chunks are re-hashed; only missing or corrupt chunks are fetched.

### Index updates

`gale update` fetches the Homebrew formula JSON snapshot with conditional GET (`If-None-Match`). Changed entries are diffed in memory against the cached index — a pragmatic approximation of binary diff since Homebrew only exposes full snapshots.

## Commands

| Command | Description |
|---------|-------------|
| `gale install <pkg>...` | Resolve deps, confirm tree, download+install |
| `gale remove <pkg>` | Mark package removed |
| `gale update` | Refresh package index |
| `gale upgrade [pkg]` | Upgrade installed packages |
| `gale search <query>` | Fuzzy search packages |
| `gale list` | Show installed packages |
| `gale info <pkg>` | Package details |
| `gale doctor` | Verify store integrity and symlinks |

### Global flags

- `--no-tui` — plain stdout logs (CI/scripting)
- `--jobs N` — max parallel download workers (default 4)
- `--verbose` — debug logging to stderr

## Benchmarks: gale vs Homebrew

Measured on Apple Silicon macOS (arm64), Homebrew 6.0.7, using [hyperfine](https://github.com/sharkdp/hyperfine) (3–10 runs, warmup where noted). Cold installs uninstall the package before each run; gale uses a fresh `GALE_HOME` with a pre-cached index (fair comparison — brew also uses cached metadata).

| Benchmark | gale | Homebrew | Winner | Speedup |
|-----------|------|----------|--------|---------|
| **CLI startup** (`version`) | 8.4 ms | 150.8 ms | gale | **18.0×** |
| **List installed** | 17.9 ms | 32.8 ms | gale | **1.8×** |
| **Package info** (`info tree`) | 18.2 ms | 1.35 s | gale | **74×** |
| **Cold install: tree** (small) | 1.11 s | 2.48 s | gale | **2.2×** |
| **Cold install: jq** (small) | 2.20 s | 2.95 s | gale | **1.3×** |
| **Cold install: fd** (medium) | 1.60 s | 2.96 s | gale | **1.9×** |
| **Cold install: bat** (medium) | 6.67 s | 4.12 s | brew | 0.6× |
| **Cold install: kubernetes-cli** (large) | 3.13 s | 3.69 s | gale | **1.2×** |

**Takeaways**

- **CLI latency** is gale's biggest win — native Go binary vs Ruby startup. `gale info` reads a local msgpack index; `brew info` can hit the network.
- **Cold installs** are mixed: gale's parallel chunked downloads win on most packages, but brew's mature pour/extract path occasionally edges it (e.g. `bat`).
- Numbers vary with network, disk cache, and bottle size. Re-run locally with `./scripts/benchmark.sh`.

### Performance optimizations (v1.1)

| Area | Change | Effect |
|------|--------|--------|
| Downloads | HTTP/2 connection pool, 1MB copy buffers, per-worker file handles | Fewer syscalls, better throughput |
| Downloads | Batched sidecar writes (750ms debounce) | Less JSON I/O during parallel chunks |
| Downloads | Skip GHCR `HEAD`, default `--jobs` = `NumCPU` | One fewer round-trip; more parallelism |
| Install | Symlink prefix → store (skip full tree clone) | Near-instant install from cache |
| Install | [klauspost/gzip](https://github.com/klauspost/compress) + system `tar` fallback | Faster bottle extraction |
| Install | Skip re-download when bottle `.tar.gz` already verified | Instant re-install |
| Metadata | SQLite WAL + `synchronous=NORMAL` | Faster install bookkeeping |

```bash
brew install hyperfine
make build
./scripts/benchmark.sh
```

## Development

```bash
make build   # build bin/gale
make test    # go test ./...
make lint    # go vet (+ golangci-lint if installed)
```

## v1 limitations

- Upgrade uses full resumable download when no prior bottle is cached locally (binary diff upgrade is future work)
- Homebrew is a read-only upstream; gale never modifies `/opt/homebrew`
- Default install prefix is `~/.gale/opt` with symlinks in `~/.gale/bin` (add to PATH)

## License

MIT
