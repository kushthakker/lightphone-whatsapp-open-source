# Third-party notices

This project depends on third-party software that retains its own license.

- [Light SDK](https://github.com/lightphone/light-sdk), MIT License, copyright
  2026 The Light Phone. The SDK source and development signing key are not
  vendored here; release builds pin an upstream SDK commit.
- [whatsmeow](https://github.com/tulir/whatsmeow), Mozilla Public License 2.0.
  It is consumed as an unmodified Go module dependency.
- [libsignal-protocol-go](https://github.com/tulir/libsignal-protocol-go), GNU
  General Public License v3.0. It is linked transitively into the server through
  whatsmeow. Project-owned source is MIT-licensed and GPL-compatible. The
  statically linked server binary is a combined work subject to GPLv3;
  distributing that binary, including inside a container, triggers GPLv3
  distribution and corresponding-source requirements for the covered work.
  This repository does not publish a prebuilt server image.
- [go-sqlite3](https://github.com/mattn/go-sqlite3), MIT License, copyright
  Yasuhiro Matsumoto.

The reviewed Go dependency report is in
[GO_DEPENDENCY_LICENSES.md](docs/GO_DEPENDENCY_LICENSES.md). Android
dependencies are fixed by the pinned Light SDK build recipe and retain their
upstream licenses.
