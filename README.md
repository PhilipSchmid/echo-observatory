# Echo Observatory — Multi-protocol network diagnostics

[![CI](https://github.com/philipschmid/echo-observatory/actions/workflows/ci.yaml/badge.svg)](https://github.com/philipschmid/echo-observatory/actions/workflows/ci.yaml) [![Docker](https://github.com/philipschmid/echo-observatory/actions/workflows/docker.yaml/badge.svg)](https://github.com/philipschmid/echo-observatory/actions/workflows/docker.yaml) [![Release](https://github.com/philipschmid/echo-observatory/actions/workflows/release.yaml/badge.svg)](https://github.com/philipschmid/echo-observatory/actions/workflows/release.yaml)

A Go service for testing network paths, proxies, and load balancers. Connect over
HTTP, HTTPS, HTTP/3, TCP, or gRPC to inspect the receiving server and request.
WebSocket connections echo text and binary messages. IPv4 and IPv6 requests
report the received client address and IP family in the UI and JSON.
An optional [trusted PROXY protocol mode](docs/configuration.md#trusted-proxy-protocol)
preserves original client addresses behind a TCP proxy while TLS stays in the app.

## Connect to Echo Observatory

```mermaid
%%{init: {"themeVariables": {"fontFamily": "Arial", "edgeLabelBackground": "#f5f8fe"}, "flowchart": {"nodeSpacing": 24, "rankSpacing": 48, "padding": 18, "curve": "bumpY"}}}%%
flowchart TB
    accTitle: Echo Observatory connection map
    accDescr: HTTP/1.1 and HTTP/2.0 clients use exposed HTTP port 80 or HTTPS port 443. Plain HTTP/2.0 requires --h2c. WebSocket uses HTTP/1.1 on either port, and the browser UI is available at /ui/. HTTP/3 uses UDP 443, TCP clients use 9090, gRPC clients use 50051, and Prometheus and Kubernetes probes use the admin listener on 3000.

    web("<b>Web clients</b><br/>browser · curl")
    socket("<b>TCP</b><br/>nc · socket clients")
    rpc("<b>gRPC</b><br/>grpcurl · SDKs")
    ops("<b>Operations</b><br/>Prometheus · Kubernetes")

    subgraph app["Echo Observatory"]
        http("<b>HTTP</b><br/>80 / TCP<br/>HTTP/1.1 · HTTP/2.0<br/>/ · /ws · /ui/<br/>HTTP/2.0 needs --h2c")
        https("<b>HTTPS</b><br/>443 / TCP<br/>HTTP/1.1 · HTTP/2.0<br/>/ · /ws · /ui/<br/>JSON metadata")
        quic("<b>HTTP/3</b><br/>443 / UDP<br/>HTTP/3 · QUIC<br/>/ · /ui/<br/>JSON metadata")
        tcp("<b>TCP</b><br/>9090 / TCP<br/>JSON metadata<br/>Close after response<br/>Native listener")
        grpc("<b>gRPC</b><br/>50051 / TCP<br/>Echo · reflection<br/>Health Check · Watch<br/>TLS in profiles")
        admin("<b>Admin</b><br/>3000 / TCP<br/>/metrics<br/>/health · /ready<br/>Private listener")
    end

    web --> http
    web --> https
    web --> quic
    socket --> tcp
    rpc --> grpc
    ops --> admin

    classDef client fill:#f5f8fe,stroke:#9fb5d5,color:#344e73,stroke-width:1.5px,rx:12px,ry:12px;
    classDef listener fill:#edf3ff,stroke:#8aa7d6,color:#243e69,stroke-width:1.5px,rx:12px,ry:12px;
    class web,socket,rpc,ops client;
    class http,https,quic,tcp,grpc,admin listener;
    style app fill:#fafcff,stroke:#d5e1f2,color:#263f66,stroke-width:1px,rx:16px,ry:16px;
    linkStyle default stroke:#819bc1,stroke-width:2px;
```

The map shows the standard exposed deployment ports.
The embedded UI uses `/ui/` on HTTP, HTTPS and HTTP/3; `/` returns JSON.
WebSocket upgrades use HTTP/1.1 on `/ws`. gRPC stays on its own port.
For public deployments, set `--public-url` to enable [search discovery](docs/usage.md#search-discovery).

## Quick start

Build and start the on-prem profile:

```bash
docker compose up --build -d
```

Open [localhost/ui/](http://localhost/ui/) to inspect your request and its network
layers, or [Workbench](http://localhost/ui/workbench/) to test HTTP and WebSocket.
HTTPS and HTTP/3 use a self-signed demo certificate until you supply
trusted certificates; see [usage](docs/usage.md#use-certificates-and-mutual-tls).

```bash
curl -sS http://localhost/
curl -sk https://localhost/   # Demo certificate only.
curl -fsS http://localhost:3000/ready
```

The container runs as UID/GID `65532`, with a read-only filesystem and all
capabilities dropped. Admin port 3000 is published only on loopback.
For binary builds and unprivileged development ports, see [development](docs/development.md).
Self-hosted defaults impose no rate or daily transfer quota; public instances
can opt in with [`--protection`](docs/configuration.md#abuse-protection).

## Browser UI

Try the [live demo](https://echo.container.education/ui/) to inspect your request,
TLS connection and TCP/IP layers, or send a request from the workbench.

![Your request and connection layers in dark mode](docs/images/request-dark.png)

<details>
<summary>Workbench: a JSON POST with a custom header, HTTP 201 and a 150 ms delay</summary>

The sample checkout payload is synthetic. The response shows request metadata,
the negotiated protocol and measured duration.

![Workbench sending a checkout request and receiving HTTP 201 over HTTP/2](docs/images/workbench-dark.png)

</details>

## Example response

An abridged HTTPS response with `--tls --print-http-request-headers`:

```json
{
  "hostname": "echo-observatory-demo",
  "listener": "TLS",
  "source_ip": "172.17.0.1",
  "ip_version": "IPv4",
  "http_version": "HTTP/2.0",
  "http_method": "GET",
  "http_host": "echo.example.test",
  "http_request_uri": "/checkout?test=42",
  "remote_addr": "172.17.0.1:56536",
  "local_addr": "172.17.0.2:18443",
  "tls_version": "TLS 1.3",
  "tls_server_name": "localhost",
  "tls_negotiated_protocol": "h2",
  "headers": {
    "Authorization": [
      "[REDACTED]"
    ],
    "X-Forwarded-For": [
      "127.0.0.1"
    ]
  }
}
```

See which backend answered, the request it received, and the protocol negotiated
on that connection. By default, `source_ip` is the socket peer (here, the proxy),
while forwarded HTTP headers remain untrusted and `Authorization` is redacted.
Trusted PROXY protocol mode reports the original client in `source_ip` and
preserves the transport peer in separate `socket_peer_*` fields.

## Endpoints at a glance

| Listener | Exposed port | Internal default | Connect to |
| --- | --- | --- | --- |
| HTTP | `80/TCP` | `8080/TCP` | JSON; `/ws` for WebSocket; `/ui/` for the UI |
| HTTPS | `443/TCP` | `8443/TCP` | JSON; `/ws` for WSS; `/ui/` for the UI |
| HTTP/3 | `443/UDP` | `4433/UDP` | JSON; `/ui/` for the UI; no WebSocket |
| TCP | `9090/TCP` | `9090/TCP` | One JSON metadata response followed by EOF |
| gRPC | `50051/TCP` | `50051/TCP` | Echo, standard Health and reflection; TLS in deployment profiles |
| Admin | **Private** `3000/TCP` | `3000/TCP` | `/metrics`, `/health`, `/ready` |

The [Compose](compose.yaml) and [Kubernetes](docs/kubernetes.md) profiles use
these exposed ports and enable all echo listeners.
The bare binary starts only HTTP and admin by default; other listeners remain
opt-in. All bind ports are configurable. Existing 1.x image paths are unchanged.

## Version 2

See the [latest stable release](https://github.com/PhilipSchmid/echo-observatory/releases/latest)
for downloads and the matching container image tag. For repeatable container
deployments, pin the image by digest.

Version 2.1 adds the [Grafana operations dashboard](dashboards/README.md),
certificate validity metrics, and rate-limit headroom and usage metrics.

Version 2.0 adds the browser UI, automatic certificate management, and the
renamed executable. Images starting with 2.0.0 use
`ghcr.io/philipschmid/echo-observatory`;
existing 1.x images remain at `ghcr.io/philipschmid/echo-app`.
See [upgrading to 2.0](docs/migration-v2.md) for configuration and metrics changes.

## Guides

- [Usage](docs/usage.md): browser UI, connection examples, response controls,
  certificates, WebSocket, health checks, and metrics.
- [Configuration](docs/configuration.md): flags, environment variables, defaults,
  and validation.
- [Monitoring](docs/monitoring.md): private health endpoints, metric reference,
  and PromQL examples.
- [Grafana dashboard](dashboards/README.md): portable operations dashboard,
  import instructions and a manual update target.
- [Privacy](https://echo.container.education/ui/privacy/): the public demo's data
  handling and contact details. Self-hosted operators can review
  [privacy and logging](docs/usage.md#privacy-and-logging).
- [Kubernetes](docs/kubernetes.md): deployment, services, probes, and pod security.
- [Development](docs/development.md): builds, tests, integration checks, and images.

Runs on-premises and offline too: use the default self-signed certificate or
supply your own PEM certificate and key. ACME is optional.

Keep the admin listener private and use synthetic data. Common credential headers
are redacted by default; `--unsafe-headers` reveals them in returned diagnostics.

[Apache 2.0 license](LICENSE).
