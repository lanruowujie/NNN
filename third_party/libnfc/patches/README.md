# libnfc patches

NFCX carries narrowly scoped compatibility patches:

- `0001-list-env-devices-without-conffiles.patch` makes `LIBNFC_DEVICE` work when filesystem configuration support is disabled. libnfc 1.8.0 populates the environment device independently, but incorrectly consumes the shared device array only inside a `CONFFILES` guard.
- `0002-macos-uart-synchronous-send.patch` adopts the narrowly scoped macOS send behavior from upstream PR [#633](https://github.com/nfc-tools/libnfc/pull/633). It sends UART data one byte at a time with a 9 microsecond pause so the FT232RL receives a complete frame before libnfc waits for the PN532 ACK.
- `0003-modern-mingw-environment-compat.patch` replaces libnfc's unsafe fixed-size Windows environment helper with MinGW-w64's `_putenv_s` and avoids errno macro redefinition warnings.
- `0004-modern-mingw-native-bitness.patch` lets CMake use the selected MinGW compiler's native target bitness instead of forcing 32-bit flags on a 64-bit build.
- `0005-windows-uart-query-dos-device.patch` enumerates current Windows COM mappings with `QueryDosDeviceA`; this covers USB serial ports that can be opened explicitly but are omitted by libnfc 1.8.0's `GetDefaultCommConfig` probe.

If another upstream patch becomes necessary, place it in this directory and add its filename to `series` in application order. Each patch must contain an upstream source, issue or pull-request URL and a short explanation of why NFCX needs it. The build fails when a listed patch is absent or cannot be applied cleanly.
