#!/usr/bin/env bash
# Create a throwaway environment for recording the demo: its own daemon socket,
# config/state dirs, and a small Go repo for the jobs to actually run against.
#
# Source this file; it exports the env vars and sets DEMO_WORKDIR.
set -euo pipefail

DEMO_ROOT="${DEMO_ROOT:-/tmp/runic-demo}"
export DEMO_WORKDIR="$DEMO_ROOT/app"

rm -rf "$DEMO_ROOT"
mkdir -p "$DEMO_ROOT/home" "$DEMO_ROOT/config" "$DEMO_ROOT/state" "$DEMO_WORKDIR"

export HOME="$DEMO_ROOT/home"
export XDG_CONFIG_HOME="$DEMO_ROOT/config"
export XDG_STATE_HOME="$DEMO_ROOT/state"
export RUNIC_SOCKET="$DEMO_ROOT/runic.sock"
export SHELL=/bin/bash
export RUNIC_SLOTS=2
export RUNIC_THEME=dark
export COLORTERM=truecolor
export TERM=xterm-256color
unset RUNIC_SESSION RUNIC_PANE || true

# Shells runic spawns inside panes read this; keep their prompt as plain as the
# recording shell's.
cat >"$HOME/.bashrc" <<'EOF'
PS1='$ '
HISTFILE=
unset PROMPT_COMMAND
EOF

cat >"$DEMO_WORKDIR/go.mod" <<'EOF'
module example.com/app

go 1.26.1
EOF

mkdir -p "$DEMO_WORKDIR/cmd/app" "$DEMO_WORKDIR/internal/store"

cat >"$DEMO_WORKDIR/cmd/app/main.go" <<'EOF'
package main

import (
	"fmt"

	"example.com/app/internal/store"
)

func main() {
	s := store.New()
	s.Put("hello", "world")
	fmt.Println(s.Get("hello"))
}
EOF

cat >"$DEMO_WORKDIR/internal/store/store.go" <<'EOF'
package store

// Store is a tiny in-memory key/value map.
type Store struct {
	data map[string]string
}

func New() *Store {
	return &Store{data: map[string]string{}}
}

func (s *Store) Put(k, v string) { s.data[k] = v }

func (s *Store) Get(k string) string { return s.data[k] }

func (s *Store) Len() int { return len(s.data) }
EOF

cat >"$DEMO_WORKDIR/internal/store/store_test.go" <<'EOF'
package store

import "testing"

func TestPutGet(t *testing.T) {
	s := New()
	s.Put("k", "v")
	if got := s.Get("k"); got != "v" {
		t.Fatalf("Get(k) = %q, want %q", got, "v")
	}
}

func FuzzPut(f *testing.F) {
	f.Add("k", "v")
	f.Fuzz(func(t *testing.T, k, v string) {
		s := New()
		s.Put(k, v)
		if got := s.Get(k); got != v {
			t.Fatalf("Get(%q) = %q, want %q", k, got, v)
		}
	})
}

func TestLen(t *testing.T) {
	s := New()
	s.Put("a", "1")
	s.Put("b", "2")
	if got := s.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
}
EOF

# A check that genuinely exits non-zero, so the demo has a real failed job.
mkdir -p "$DEMO_WORKDIR/scripts"
cat >"$DEMO_WORKDIR/scripts/lint.sh" <<'EOF'
#!/usr/bin/env bash
set -uo pipefail

echo "gofmt..."
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "not gofmt'd:"
	echo "$unformatted"
	exit 1
fi

echo "checking for leftover TODOs..."
if grep -rn "TODO" --include="*.go" .; then
	echo
	echo "lint: unresolved TODOs in the tree"
	exit 1
fi
EOF
chmod +x "$DEMO_WORKDIR/scripts/lint.sh"

# The TODO the lint check trips on.
cat >>"$DEMO_WORKDIR/internal/store/store.go" <<'EOF'

// TODO: persist the map to disk between runs.
EOF

# Two long-running jobs, so the demo has something in RUNNING and QUEUED and
# live output to follow in the pager.
cat >"$DEMO_WORKDIR/scripts/bench.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

echo "goos: linux"
echo "goarch: amd64"
echo "pkg: example.com/app/internal/store"
echo "cpu: 12 cores"
echo
i=0
for name in Put Get Len PutParallel GetParallel Mixed Grow Delete; do
	for procs in 1 8; do
		i=$((i + 1))
		sleep 7
		printf 'Benchmark%s-%-2d %10d\t%8.1f ns/op\t%3d B/op\t%d allocs/op\n' \
			"$name" "$procs" $((i * 137000)) \
			"$(awk -v i="$i" 'BEGIN{print 38.6 + i * 1.7}')" $((16 + i % 3)) 1
	done
done
echo
echo "PASS"
EOF
chmod +x "$DEMO_WORKDIR/scripts/bench.sh"

cat >"$DEMO_WORKDIR/scripts/e2e.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

scenarios=(
	"cold start"
	"write 10k keys"
	"read-your-writes"
	"restart under load"
	"concurrent writers"
	"large values"
	"key eviction"
	"snapshot + restore"
	"corrupt snapshot"
	"graceful shutdown"
)

failures=0
for scenario in "${scenarios[@]}"; do
	echo "=== $scenario"
	for step in setup exercise verify; do
		sleep 3
		printf '    %-10s ok\n' "$step"
	done
done
echo
echo "${#scenarios[@]} scenarios, $failures failures"
EOF
chmod +x "$DEMO_WORKDIR/scripts/e2e.sh"

cat >"$DEMO_WORKDIR/scripts/fuzz.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

echo "fuzz: seeding corpus"
sleep 5
go test -run XXX -fuzz FuzzPut -fuzztime 20s ./internal/store/ 2>&1 || true
EOF
chmod +x "$DEMO_WORKDIR/scripts/fuzz.sh"

cat >"$DEMO_WORKDIR/Makefile" <<'EOF'
build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint:
	./scripts/lint.sh

bench:
	./scripts/bench.sh

e2e:
	./scripts/e2e.sh

fuzz:
	./scripts/fuzz.sh

docs:
	go doc -all ./internal/store
EOF

cd "$DEMO_WORKDIR"
git init -q -b main
git config user.email demo@example.com
git config user.name "Runic Demo"
git add -A
git commit -qm "Initial commit"

echo "sandbox ready: $DEMO_WORKDIR (socket $RUNIC_SOCKET)"
