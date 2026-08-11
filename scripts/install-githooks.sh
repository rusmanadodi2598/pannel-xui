#!/usr/bin/env bash
# install-githooks.sh — aktivasi git hooks + verifikasi gitleaks (idempotent).
#
#   bash scripts/install-githooks.sh
#
# Yang dilakukan:
#   1. Cek gitleaks tersedia; bila tidak, tawarkan install via `go install`.
#   2. chmod +x semua hook di .githooks/.
#   3. Cek golangci-lint (opsional; CI tetap enforce).
#   4. git config core.hooksPath .githooks  (per-clone; versi lain: pre-commit
#      framework — lihat bot/README.md bagian Security & Git Hooks).
set -eu

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "=== [1/4] gitleaks ==="
if command -v gitleaks >/dev/null 2>&1; then
  echo "gitleaks tersedia: $(gitleaks version 2>&1 | head -1)"
else
  echo "gitleaks TIDAK ditemukan."
  echo "Install via: go install github.com/gitleaks/gitleaks/v8@latest"
  echo "  (pastikan \$GOBIN/\$GOPATH/bin ada di PATH), lalu jalankan ulang skrip ini."
  exit 1
fi

echo
echo "=== [2/4] chmod hooks ==="
for h in .githooks/pre-commit .githooks/commit-msg .githooks/pre-push; do
  if [ -f "$h" ]; then
    chmod +x "$h"
    echo "  +x $h"
  else
    echo "  ⚠️  $h tidak ditemukan — skip"
  fi
done

echo
echo "=== [3/4] golangci-lint (opsional, lebih ketat dari go vet) ==="
if command -v golangci-lint >/dev/null 2>&1; then
  echo "golangci-lint tersedia: $(golangci-lint version 2>&1 | head -1 | awk '{print $3}')"
else
  echo "golangci-lint TIDAK ditemukan — pre-commit melewati lint bot/ (CI tetap enforce)."
  echo "Install opsional (PIN versi v1.64.x — config .golangci.yml format v1):"
  echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8"
  echo "  (config: .golangci.yml di root repo)"
fi

echo
echo "=== [4/4] aktivasi core.hooksPath ==="
git config core.hooksPath .githooks
echo "  core.hooksPath -> $(git config --get core.hooksPath)"

echo
echo "✅ Git hooks aktif. Verifikasi cepat:"
echo "   - pre-commit : gitleaks protect --staged + gofmt + golangci-lint (bot/ baris baru) + whitespace"
echo "   - commit-msg : Conventional Commits (feat|fix|docs|ci|...) "
echo "   - pre-push   : gitleaks scan commit baru + go build/vet (root & bot) + golangci-lint bot/"
echo
echo "Tes manual (opsional):"
echo "   bash .githooks/pre-commit          # tanpa staging → lolos"
echo "   echo 'test: x' > /tmp/msg && bash .githooks/commit-msg /tmp/msg"
