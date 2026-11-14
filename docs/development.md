# Development

[Back to README](../README.md) · [Configuration](configuration.md)

## Requirements

- Go 1.26.9 or newer, with a current security patch.
- Make and Bash for the repository targets.
- Node.js 22 or newer for `make test-ui`; no npm packages are needed.
- golangci-lint v2; `make tools` installs the CI version, v2.13.2.
- Docker with Buildx for container builds.
- curl and jq for `scripts/test-image.sh`.

`make tools` also installs the Go protobuf plugins. Install `protoc` separately
only when changing `proto/echo.proto`.

## Build and run

The Go module is `github.com/PhilipSchmid/echo-observatory/v2`, following
[Go's major-version import convention](https://go.dev/doc/modules/major-version).
The executable and release archives are named `echo-observatory`.

The footer shows the running binary's build version. Make targets stamp the Git
version; override it with `VERSION=v2.1.1` when preparing a release. Direct Docker
builds accept `--build-arg VERSION=v2.1.1`. Unstamped source builds show `dev`,
while `go install` uses its module version when available.

```bash
make deps          # Download dependencies and verify their checksums
make build-quick   # Build ./echo-observatory without checks
make run           # HTTP and admin listeners
```

Rebuild after changing Go source or UI assets: the `make run*` targets start the
existing binary.
Use `make run-all` for HTTP, HTTPS, TCP, gRPC, and HTTP/3, or run the binary with
the [configuration flags](configuration.md).
For a fast local preview without Docker:

```bash
make dev
```

Open [localhost:18880/ui/](http://localhost:18880/ui/) to inspect your request, or
[Workbench](http://localhost:18880/ui/workbench/) to test HTTP and WebSocket.
[Air](https://github.com/air-verse/air)
automatically rebuilds and restarts the Go process when Go source or embedded UI
assets change. Refresh the browser after the restart. Stop it with Ctrl+C.
The pinned development tool downloads on the first run and is cached by Go;
it adds no application dependency. For offline development, run it once before
disconnecting.

The development profile uses unprivileged ports: HTTP 18880, HTTPS and HTTP/3
18843 (TCP/UDP), TCP echo 19990, gRPC TLS 15051, and admin 13300. These override
the binary defaults only for `make dev`; standard deployment ports stay the same.
HTTPS uses a generated self-signed certificate. Listeners bind to the machine's
interfaces, so use a trusted development network. Air's browser-injection proxy
stays disabled to preserve the UI's Content Security Policy and direct connection
diagnostics.

The UI's HTML, CSS, and JavaScript are embedded in the Go binary with `go:embed`.
There is no frontend toolchain or separate asset build.
Static assets share precomputed gzip and ETags. Browsers revalidate them on each
use, so Air rebuilds produce fresh assets on refresh. Request details and UI
settings remain uncached.
The UI uses locally served [Nunito](https://github.com/google/fonts/tree/main/ofl/nunito)
and [Sometype Mono](https://github.com/google/fonts/tree/main/ofl/sometypemono)
variable WOFF2 fonts. Their SIL Open Font License notices are bundled with the
assets ([Nunito](../internal/ui/assets/Nunito-OFL.txt),
[Sometype Mono](../internal/ui/assets/SometypeMono-OFL.txt)); visitors never
contact an external font service.

The GitHub mark is from [Octicons](https://github.com/primer/octicons), with its
[MIT license](../internal/ui/assets/Octicons-LICENSE.txt) included in the assets.

`make build` runs lint, vet, and coverage tests before building. For tests with
the race detector, use `make test` or `make check`.

## Checks

```bash
go mod tidy
git diff --exit-code -- go.mod go.sum
make check
make test-ui
make test-integration
```

`make check` runs lint with errcheck, vet, and race tests. Unit tests also parse
the Compose and documented Kubernetes definitions to prevent exposed-port
and private-admin mappings from drifting. Integration tests build
a temporary application binary and exercise HTTP/1.1 and HTTP/2.0 over plain
HTTP (h2c for HTTP/2.0) and HTTPS, plus HTTP/3, TCP, and gRPC connections.
They also cover response controls, WebSocket echo, readiness transitions, and
shutdown. They verify the UI and its assets over all HTTP versions, UI opt-out
over HTTP/3, HTTPS discovery headers, and HTTPS/HTTP3 sharing one TCP/UDP port
number. All protocol and WebSocket checks run against IPv4 and IPv6 loopback;
the IPv6 subtest explicitly skips only if the OS has no IPv6 loopback listener.
No curl HTTP/3 support or grpcurl installation is needed for this suite.
Results are saved in
`build/test-results.log`.

`make test-ui` uses Node.js 22 or newer's built-in test runner for the browser
client's URL handling, example presets, copied commands, and clipboard feedback.
It needs no npm packages and runs in CI. Building and running Echo Observatory
requires only Go.

For checks that match CI, use a patched Go 1.26 toolchain and golangci-lint
v2.13.2. If you build with a newer Go release, use a linter that supports it.

Useful focused commands:

| Command | Purpose |
| --- | --- |
| `go test -race ./internal/server -run TestHTTPServer` | Run HTTP server tests. |
| `make test-short` | Run tests with Go's short-test flag. |
| `mkdir -p build && make test-coverage` | Write `build/coverage.html`. |
| `make benchmark` | Run Go benchmarks with allocation statistics. |
| `make fmt` | Format Go files. |
| `make lint-fix` | Apply supported linter fixes. |
| `make proto` | Regenerate protobuf sources after schema edits. |

CI also runs staticcheck and the vulnerability scanner:

```bash
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

Use a patched Go toolchain. CI checks module tidiness and formatting, runs race
and wire tests, and builds five platform targets. The container build uses
the toolchain pinned in the Dockerfile.

### Accessibility

The UI targets [WCAG 2.2 AA](https://www.w3.org/TR/WCAG22/). Keep the rendered-HTML
semantic checks in `go test -race ./internal/ui` and the keyboard, feedback and
contrast regressions in `make test-ui` passing when changing UI components.

Before release, check every page:

- Use only the keyboard: follow the skip link, operate dropdowns and tabs, dismiss
  help with Escape, submit invalid fields, and copy a command. Check focus order,
  visible focus, field errors and status feedback.
- Check both themes at 320 CSS pixels wide and with text enlarged to 200%.
  Apply text-spacing overrides: 1.5 line height, 2em paragraph spacing, 0.12em
  letter spacing and 0.16em word spacing. Controls and text must remain usable.
- Enable reduced motion and forced colors. Information and focus must remain
  clear without animation or color alone.
- Test with VoiceOver on macOS and NVDA on Windows: verify names, reading order,
  expanded/selected states, errors and status announcements.

Automated checks do not establish conformance. Record the actual browser and
assistive-technology results before claiming accessibility compliance.

## Cross-platform binaries

```bash
make build-all
```

Binaries are written to `build/` for Linux amd64/arm64, macOS amd64/arm64, and
Windows amd64. Override `BUILD_DIR` to choose another output directory.

To install the executable into Go's binary directory:

```bash
go install ./cmd/echo-observatory
```

## Containers

For the standard public/on-prem port layout, run `docker compose up --build -d`.
This builds the current checkout, keeps the server non-root, maps 80/TCP and
443/TCP+UDP, and binds admin 3000 only to loopback. For an offline runtime, build
or import the image before disconnecting, then use `docker compose up --no-build`.

```bash
make docker
bash scripts/test-image.sh ghcr.io/philipschmid/echo-observatory:latest
```

The smoke test starts a non-root container with a read-only filesystem and all
capabilities dropped, checks HTTP/admin endpoints, and verifies clean shutdown.

`make docker-multi` builds Linux amd64/arm64 images with Buildx without publishing
them; output availability depends on the selected Buildx driver.
`make docker-push` publishes the locally available version and `latest` tags;
run it only when publication is intended.

The image workflow publishes branch/SHA tags after checks pass. The release
workflow owns version tags and stable `latest`; both test an image before publication.
Prereleases publish only their full version tag, leaving stable aliases alone.
Pin `2.1.1` or an image digest for repeatable deployments; `latest` follows the
most recent stable release.
Starting with 2.0.0, releases use `ghcr.io/philipschmid/echo-observatory`.
Legacy 1.x tags retain `ghcr.io/philipschmid/echo-app`.

Stable releases from 2.0 onward in `PhilipSchmid/echo-observatory` deploy to the
existing Fly.io app after both the image and GitHub release are published. The
deployment uses the image digest, preserves the live Fly configuration, and checks
every machine and the version at
`https://echo.container.education/ui/`. Prereleases and legacy releases do not deploy.
Automatic deployment is disabled in forks. The repository secret `FLY_API_TOKEN`
must contain a deploy token scoped to `echo-observatory`; renew it before its
expiry. The app's VM configuration must allow its non-root process to bind UDP
port 443 for HTTP/3; Fly requires the
[same internal and external UDP port](https://fly.io/docs/networking/udp-and-tcp/).
This is persisted in Fly's app configuration:

```toml
[[vm]]
kernel_args = ['sysctl.net.ipv4.ip_unprivileged_port_start=443']
```

Retain this setting alongside the existing VM size fields. It replaces the old
Fly-specific image's root startup wrapper; the published image runs directly as
UID 65532. The deployment script reads the live configuration and refuses to
replace any image if this setting is absent or changed. This check requires
Python 3.11+ for its standard-library TOML parser.

Maintainers can deploy an already published image by setting `FLY_API_TOKEN` and
running
`bash scripts/deploy-fly.sh v2.1.2 sha256:<published-image-digest>`.
Run `node --test tests/deploy-fly.test.mjs` to check deployment failure handling.

## Contributing

Keep changes focused and include a regression test for new behavior or bug fixes.
Run the checks above before opening a pull request. Protocol changes need wire
tests; UI changes need keyboard and responsive-layout checks as well as unit tests.
When adding a flag, update the loader default and configuration reference together;
`TestFlagDefaultsMatchConfiguration` checks that CLI and loader defaults agree.
Keep external services opt-in and UI assets bundled so self-hosted installations
can run offline.

Run `make help` for the complete target list. `make clean` removes local binaries,
the build directory, and coverage files, and clears the Go test cache.
