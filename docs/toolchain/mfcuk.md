# MFCUK toolchain

- Upstream: <https://github.com/nfc-tools/mfcuk>
- Tag: `mfcuk-0.3.8`
- Commit: `0c5aff4a1b31074a8418b4bcb1b4b9bd6df477fd`
- Source archive: <https://github.com/nfc-tools/mfcuk/archive/refs/tags/mfcuk-0.3.8.tar.gz>
- License: GPL-2.0-or-later

Machine-readable source identity is stored in `third_party/mfcuk/mfcuk.lock`.
The build verifies the archive SHA-256 before extraction and links against the
same pinned libnfc 1.8.0 SDK used by NFCX and MFOC.

NFCX applies `0001-nfcx-runtime-integration.patch`. It removes an obsolete
configure-time requirement for Linux/BSD/macOS endian headers so MinGW-w64 can
use mfcuk already implemented GCC byte-swap builtins. It also replaces legacy
`WIN32` guards with MinGW-w64's standard `_WIN32` macro so the existing Windows
`Sleep()`/`xgetopt` path is compiled instead of the POSIX `select()` path, and adds an
`NFCX_RESULT key=<A|B> sector=<n> value=<12 hex>` line after upstream has
completed recovery. Candidate keys are still untrusted until the in-process
libnfc reader authenticates them.

```sh
make mfcuk-check
NFCX_MFCUK_ARCHIVE=/tmp/mfcuk-0.3.8.tar.gz make mfcuk-build
make mfcuk-verify
```
