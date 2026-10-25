# List available recipes
default:
    @just --list

# ==============================================================================
# Go Directives
# ==============================================================================

# Run all Go unit and conformance tests
test:
    go test -race -v ./...

# Run only the Protobuf-style conformance & malicious wire suite (and verify testdata/conformance_corpus.json)
conformance:
    go test -v -run "TestProtobufStyleConformanceCorpus|TestSchemaEvolutionUnknownFieldsConformance|TestMalformedWireRejectionConformance|TestGoldenWireFiles" ./...

# Regenerate golden files in testdata/golden/ and testdata/conformance_corpus.json
update-golden:
    go test -v -run "TestGoldenWireFiles|TestProtobufStyleConformanceCorpus" . -update-golden

# Run the native Go fuzzer against all decoders, Opaque getters, and O(1) readers (e.g., `just fuzz 30s`)
fuzz time="10s":
    go test -fuzz=FuzzConformanceNoPanic -fuzztime={{time}} .

# Lint Go code using golangci-lint
go-lint:
    golangci-lint run ./...

# Format Go code using golangci-lint fmt (gofmt + goimports)
go-fmt:
    golangci-lint fmt

# Check Go code formatting without modifying files
go-fmt-check:
    golangci-lint fmt --diff

# Run all Go benchmarks with memory allocation stats and persist raw output to testdata/benchmarks.txt
bench:
    GOEXPERIMENT=jsonv2 go test -bench=. -benchmem . | tee testdata/benchmarks.txt

# Regenerate pb/peer_omnibuf.pb.go and pb/peer_vtproto.pb.go
generate:
    go run ./cmd/protoc-gen-omnibuf -self-gen=pb/peer_omnibuf.pb.go
    go run ./cmd/protoc-gen-omnibuf -vtproto-gen=pb/peer_vtproto.pb.go

# Build the omnibuf CLI binary into ./bin/omnibuf
build-cli:
    @mkdir -p bin
    go build -o bin/omnibuf ./cmd/omnibuf

# Build the protoc-gen-omnibuf plugin binary into ./bin/protoc-gen-omnibuf
build-plugin:
    @mkdir -p bin
    go build -o bin/protoc-gen-omnibuf ./cmd/protoc-gen-omnibuf

# ==============================================================================
# Protobuf / Buf Directives
# ==============================================================================

# Lint Protobuf schemas using buf
buf-lint:
    buf lint

# Format Protobuf schemas in place using buf
buf-fmt:
    buf format -w

# Check Protobuf schema formatting without modifying files
buf-fmt-check:
    buf format -d --exit-code

# Verify all Protobuf schemas compile cleanly using buf
buf-build:
    buf build

# Check Protobuf schemas for breaking changes against a git ref (default: main branch)
buf-breaking against=".git#branch=main":
    buf breaking --against "{{against}}"

# ==============================================================================
# Frontend & Cloudflare Pages Directives (omnibuf.com)
# ==============================================================================

# Install web dependencies via pnpm
[working-directory: 'web']
web-install:
    pnpm install --frozen-lockfile

# Start the Astro site dev server (alias: `just web`)
[working-directory: 'web']
web-dev:
    pnpm run dev

alias web := web-dev

# Build the static Astro site into web/dist
[working-directory: 'web']
web-build:
    pnpm run build

# Preview the production Astro build locally
[working-directory: 'web']
web-preview:
    pnpm run preview

# Lint and typecheck the Astro frontend (ESLint + astro check)
[working-directory: 'web']
web-lint:
    pnpm run lint

# Format the Astro frontend using Prettier
[working-directory: 'web']
web-fmt:
    pnpm run fmt

# Check Astro frontend formatting without modifying files
[working-directory: 'web']
web-fmt-check:
    pnpm run fmt:check

# Preview the built site locally under Cloudflare Pages runtime (Wrangler)
[working-directory: 'web']
web-pages-preview:
    pnpm run pages:preview

# Deploy web/dist to Cloudflare Pages (project: omnibuf, domain: omnibuf.com)
[working-directory: 'web']
web-deploy:
    pnpm run pages:deploy

# ==============================================================================
# Combined Repo-Wide Directives (Format, Lint, CI)
# ==============================================================================

# Format Go, Protobuf, and frontend code
fmt: go-fmt buf-fmt web-fmt

# Check formatting across Go, Protobuf, and frontend code
fmt-check: go-fmt-check buf-fmt-check web-fmt-check

# Lint Go, Protobuf, and frontend code
lint: go-lint buf-lint web-lint

# Run all Go CI checks (format check, lint, unit/conformance tests, and fuzzing)
ci-go fuzz_time="5s": go-fmt-check go-lint test (fuzz fuzz_time)

# Run all Protobuf CI checks (format check, lint, and build)
ci-proto: buf-fmt-check buf-lint buf-build

# Run all frontend CI checks (format check, lint/typecheck, and production build)
ci-web: web-fmt-check web-lint web-build

# Run full CI suite across Go, Protobuf, and frontend
ci fuzz_time="5s": (ci-go fuzz_time) ci-proto ci-web
