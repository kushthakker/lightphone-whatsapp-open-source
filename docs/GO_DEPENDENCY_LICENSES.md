# Go dependency licenses

Generated and reviewed on 2026-08-15 with:

```bash
cd server
go run github.com/google/go-licenses@v1.6.0 report ./...
```

Project-owned packages are covered by the repository MIT license. Upstream Go
packages used by the server are:

| Module | License |
| --- | --- |
| `filippo.io/edwards25519` | BSD-3-Clause |
| `github.com/beeper/argo-go` | MIT |
| `github.com/coder/websocket` | ISC |
| `github.com/elliotchance/orderedmap/v3` | MIT |
| `github.com/google/uuid` | BSD-3-Clause |
| `github.com/mattn/go-colorable` | MIT |
| `github.com/mattn/go-isatty` | MIT |
| `github.com/mattn/go-sqlite3` | MIT |
| `github.com/petermattis/goid` | Apache-2.0 |
| `github.com/rs/zerolog` | MIT |
| `github.com/skip2/go-qrcode` | MIT |
| `github.com/vektah/gqlparser/v2` | MIT |
| `go.mau.fi/libsignal` | GPL-3.0 |
| `go.mau.fi/util` | MPL-2.0 |
| `go.mau.fi/whatsmeow` | MPL-2.0 |
| `golang.org/x/crypto` | BSD-3-Clause |
| `golang.org/x/exp` | BSD-3-Clause |
| `golang.org/x/net` | BSD-3-Clause |
| `golang.org/x/sync` | BSD-3-Clause |
| `golang.org/x/sys` | BSD-3-Clause |
| `golang.org/x/text` | BSD-3-Clause |
| `google.golang.org/protobuf` | BSD-3-Clause |

The authoritative license texts remain in the exact module versions selected
by `server/go.mod` and `server/go.sum`. The GPL-3.0 Signal module is not a
system library. The statically linked server binary is a combined work subject
to GPLv3; distributing that binary, including inside a container, requires
making corresponding source for the covered work available. The container's
unrelated components remain an aggregate. This is a compliance note, not legal
advice.
