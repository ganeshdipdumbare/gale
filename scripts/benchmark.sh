#!/usr/bin/env bash
# Benchmark gale vs Homebrew using hyperfine.
# Usage: brew install hyperfine && ./scripts/benchmark.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GALE="$ROOT/bin/gale"
RUNS="${BENCH_RUNS:-3}"
WARMUP="${BENCH_WARMUP:-2}"

command -v hyperfine >/dev/null || { echo "install hyperfine: brew install hyperfine"; exit 1; }
[[ -x "$GALE" ]] || make -C "$ROOT" build

WARM_HOME="$ROOT/.benchmark-gale"
mkdir -p "$WARM_HOME"
GALE_HOME="$WARM_HOME" "$GALE" --no-tui update >/dev/null 2>&1 || true

echo "=== CLI benchmarks (warm cache) ==="
hyperfine --warmup "$WARMUP" -r "$RUNS" \
  -n gale "$GALE version" \
  -n brew "brew --version"

hyperfine --warmup "$WARMUP" -r "$RUNS" \
  -n gale "GALE_HOME=$WARM_HOME $GALE --no-tui list" \
  -n brew "brew list --formula"

hyperfine --warmup "$WARMUP" -r "$RUNS" \
  -n gale "GALE_HOME=$WARM_HOME $GALE --no-tui info tree" \
  -n brew "brew info tree"

bench_install() {
  local pkg="$1"
  echo "=== Cold install: $pkg ==="
  hyperfine --warmup 0 -r "$RUNS" \
    --prepare "brew uninstall --force $pkg >/dev/null 2>&1 || true; rm -rf /tmp/gale-b-$pkg && mkdir -p /tmp/gale-b-$pkg && GALE_HOME=/tmp/gale-b-$pkg $GALE --no-tui update >/dev/null" \
    -n gale "GALE_HOME=/tmp/gale-b-$pkg $GALE --no-tui install -y $pkg" \
    -n brew "brew install $pkg"
}

echo "=== Cold install benchmarks (index pre-cached for gale) ==="
for pkg in tree jq fd bat kubernetes-cli; do
  bench_install "$pkg"
done

echo "Done. Update README table with results above."
