# Contributing to PXE-in-a-Box

## Development Setup

```bash
git clone https://github.com/lenaxia/pxe-in-a-box.git
cd pxe-in-a-box
go mod download
pre-commit install
pre-commit install --hook-type pre-push
```

## Code Style

- **Go**: Follow [Effective Go](https://go.dev/doc/effective_go). Run `gofmt` and `go vet`.
- **No comments unless asked** — the code should be self-documenting.
- **TDD** — write tests first, then implementation. Every package has `_test.go` files.
- **Type safety** — no `any` where a concrete type works. No panics in normal flow.

## Test Tiers

| Tier | Build Tag | External Deps | When to Run |
|------|-----------|---------------|-------------|
| Unit | (default) | None | Every commit |
| Integration | `integration` | None (network optional) | PR |
| E2E HTTP | `e2e` | matchbox binary | PR |
| E2E QEMU | `e2e` | matchbox + QEMU | PR |

```bash
make test-unit          # fast, no deps
make test-integration   # pipeline tests, OS URL checks
make test-e2e-http      # needs matchbox on PATH
make test-e2e-qemu      # needs matchbox + qemu-system-x86_64
```

## Pull Request Checklist

- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `gofmt -l .` returns empty
- [ ] New code has test coverage
- [ ] No secrets committed (gitleaks passes)
- [ ] No hardcoded IPs or environment-specific values in committed files
- [ ] Commits follow conventional commits style

## Adding a New OS Type

1. Add the asset type to `internal/config/assets.go` (struct + `FindAsset` + `AllAssetIDs`)
2. Add URL resolution in `internal/downloader/sources.go`
3. Add boot path mapping in `internal/matchbox/generate.go` (`assetBootPaths`)
4. Add boot path mapping in `internal/bootscript/generate.go` (`bootPaths`)
5. Add kernel args in `internal/bootscript/generate.go` (`buildKernelArgs`)
6. Add URL reachability test in `internal/e2e/os_urls_integration_test.go`
7. Add to `os-verify.yml` CI workflow matrix
8. Update `examples/assets.yaml` and README

## Adding a New Template Variable

1. Add the variable to the Jinja2 template (`ansible/templates/talos/*.j2`)
2. Document it in the README template variables table
3. Add a test case in `ansible/tests/test_templates.yml`
4. Add to `examples/machines.yaml` as a comment

## Release Process

1. Update version in code/docs if needed
2. Create a tag: `git tag v1.0.0`
3. Push: `git push origin v1.0.0`
4. CI automatically builds multi-arch Docker image and creates GitHub release
