# libnfc toolchain

NFCX specification 02 pins and builds libnfc for the in-process NFC runtime that will be introduced by specification 03. This stage supports macOS arm64 and the `pn532_uart` driver only.

## Pinned source

| Field | Value |
|---|---|
| Upstream | <https://github.com/nfc-tools/libnfc> |
| Release | `libnfc-1.8.0` |
| Commit | `f02ff51449240102c27a97173dc495e8e7789046` |
| Release archive | `libnfc-1.8.0.tar.bz2` |
| SHA-256 | `6d9ad31c86408711f0a60f05b1933101c7497683c2e0d8917d1611a3feba3dd5` |
| License | LGPL-3.0-or-later |
| NFCX patches | See `third_party/libnfc/patches/series` |

The machine-readable values are in `third_party/libnfc/libnfc.lock`. The build refuses archives whose SHA-256 does not match. The upstream `COPYING` file is copied next to every generated runtime, and corresponding source is available from the recorded release URL and commit.

## Build profile

The release's Autotools build is used because the official release archive already contains `configure`. The exact NFCX options are:

```text
--prefix=<generated development SDK>
--sysconfdir=<generated development SDK>/etc
--with-drivers=pn532_uart
--enable-shared
--disable-static
--disable-conffiles
--enable-envvars
--disable-dependency-tracking
```

The script also supplies libtool's `lt_cv_sys_max_cmd_len` cache value from `getconf ARG_MAX`. This avoids libtool's older Darwin probe calling a sandbox-blocked `sysctl`; the resolved value is recorded in `build-info/configure-env.txt`.

The release `configure` script tries to derive a revision with `git describe` even though release archives have no `.git` metadata. During configuration the script masks only the `git` executable, preventing an enclosing NFCX repository revision from being embedded as the libnfc revision. Source identity instead comes exclusively from the pinned archive hash, release tag and commit in `libnfc.lock`.

Compilation runs inside the disposable extracted source tree. This keeps C `__FILE__` strings relative; an out-of-tree Autotools build would otherwise embed its private temporary source path in the dylib.

Disabling conffiles prevents NFCX from reading or depending on `/etc/nfc` and avoids embedding a developer-specific configuration path. NFCX carries a minimal patch that keeps libnfc 1.8.0's environment-defined device list active without conffiles, so an exact device can be selected with `LIBNFC_DEVICE=pn532_uart:<serial-port>`. NFCX never writes system-level libnfc configuration.

Because libnfc classifies UART probing as an intrusive scan, the desktop application supplies process-local defaults of `LIBNFC_AUTO_SCAN=true` and `LIBNFC_INTRUSIVE_SCAN=true` before creating its backend. This gives the GUI the same UART discovery behaviour as a typical `libnfc.conf` containing `allow_intrusive_scan=yes`, without reading or changing that file. An explicitly supplied environment value is preserved, so operators can disable either scan.

Because only `pn532_uart` is enabled, this profile does not require libusb, PC/SC, CCID drivers or their runtime libraries. The serial port implementation uses the operating system APIs provided by macOS.

On macOS, NFCX also carries the UART pacing portion of upstream libnfc PR [#633](https://github.com/nfc-tools/libnfc/pull/633). A full frame written asynchronously through the FT232RL can otherwise be followed by an ACK read too early; the patch sends bytes with a baud-rate-sized delay. Non-Apple UART behavior is unchanged.

## Commands

```sh
make toolchain-check
make libnfc-build
make libnfc-verify
make libnfc-smoke
make libnfc-repeatability-check
make libnfc-binding-test
```

The normal smoke test disables automatic and intrusive scans. It first verifies that an empty environment reports zero devices, then supplies a synthetic `LIBNFC_DEVICE` and verifies that it is listed without being opened. This is the regression test for the NFCX patch and does not access or write a card.

On Windows, NFCX synchronizes its default `LIBNFC_AUTO_SCAN=true` and
`LIBNFC_INTRUSIVE_SCAN=true` values into the C runtime immediately before
`nfc_init()`. Go updates the Win32 process environment at runtime, while
libnfc reads the C runtime environment through `getenv()`; those views are not
guaranteed to stay synchronized after startup. Values explicitly supplied in
the environment before NFCX starts are preserved.

The pinned Windows UART backend is also patched to discover COM ports through
`QueryDosDeviceA`. Upstream libnfc 1.8.0 uses `GetDefaultCommConfig` as its
existence probe, which can omit a USB serial port even though the same `COMx`
name opens successfully. Discovery only adds the port as a candidate: libnfc
still opens it exclusively and completes the PN532 communication check before
returning a device.

Hardware validation is always explicit:

```sh
make libnfc-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
make libnfc-binding-hardware-smoke LIBNFC_DEVICE='pn532_uart:<serial-port>'
```

The toolchain hardware command accepts only a `pn532_uart:` connstring, passes it through `LIBNFC_DEVICE`, disables autoscan and runs the pinned build's `nfc-list -v`. The binding hardware command additionally runs 100 Go Reader open/close cycles and reads UID, ATQA and SAK from an owned ISO14443A card. Neither command stores the serial port in the repository.

The Go binding uses an explicit `libnfc` build tag. Ordinary `go test ./...` builds a clear unsupported stub and therefore remains independent of CGO, the native SDK and NFC hardware. `make libnfc-binding-test` supplies the pinned SDK and runtime paths, compiles the C shim, and runs its initialization and resource-lifecycle tests.

The shim also copies `nfc_device_get_name` into Go-owned storage after a successful open. No pointer returned by libnfc leaves `internal/nfc/libnfc`. `make dev` and `make build` supply the same pinned SDK/runtime paths and explicit build tag, so the desktop application uses the real backend rather than the hardware-free stub.

## Output separation

Development files are generated under:

```text
build/toolchain/darwin-arm64/sdk/
  include/
  lib/
  lib/pkgconfig/
  bin/
```

Application runtime files are generated under `runtime/darwin-arm64/`. The macOS library ID is rewritten to `@rpath/libnfc.6.dylib`; verification rejects private build paths and non-system absolute dynamic dependencies.

Windows and Linux layouts remain `runtime/windows-amd64` and `runtime/linux-amd64`, but their native build profiles are intentionally deferred. Platform scripts must use native runners when those profiles are implemented.
