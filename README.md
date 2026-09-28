# NNN

NNN is a cross-platform GUI workbench for **authorized NFC/card read-write security testing**. Its primary purpose is to validate card data handling, authentication boundaries, dump integrity, write safety, recovery behavior, and clone-detection controls in controlled real environments and in an offline synthetic-card laboratory.

> Use NNN only with cards, readers, and systems that you own or are explicitly authorized to test. The project is not intended to bypass access controls or copy credentials for unauthorized use.

## Design logic

NNN separates the application into four layers:

```text
Wails GUI
  └─ application bindings
      ├─ device/card services
      ├─ dump/workbench services
      ├─ key and recovery workflows
      └─ task/event coordinator
          ├─ in-process libnfc reader path
          ├─ controlled external-engine adapters
          └─ offline virtual-card lab
```

The GUI never handles native reader objects directly. Device operations are serialized by a device manager. Writes pass through preflight checks, authentication, block ordering, read-back verification, and card-identity checks. Long-running work is represented as a cancellable task and reported through `nfcx:task` events.

## Runtime mechanism

1. The user selects a validated reader profile and scans a card.
2. NNN records card facts such as UID, ATQA, SAK, type, and connection state.
3. Read and write operations work through the reader abstraction; raw dumps remain separate from sidecar metadata.
4. The workbench tracks known bytes, unknown bytes, edits, access bits, and validation warnings.
5. A write operation performs capacity and structure checks, authenticates before writing, protects block 0 by default, writes in a safe order, and verifies by immediate read-back.
6. Recovery or audit workflows run as cancellable tasks. Each stage emits progress and human-readable logs to the GUI.
7. Results are retained as structured task output and are suitable for regression comparison and security reports.

## Main test targets

NNN is intended for authorized real-environment testing of:

- reader discovery, connection loss, card insertion/removal, and device ownership;
- known-key authentication and access-control behavior;
- read, edit, dump, restore, write-order, and read-back verification;
- malformed dump, access-bit, BCC, counter, and card-change handling;
- recovery workflow availability and failure containment;
- replay, stale-state, downgrade, and clone-detection defenses;
- GUI task cancellation, progress reporting, and audit-log completeness.

The project also includes an offline virtual-card lab under `tools/virtual_lab`. Its one-click audit and clone simulation use synthetic data only, are bounded to a small candidate set, and never connect to readers or accept real dumps.

## Repository map

| Path | Role |
| --- | --- |
| `app/` | Wails bindings, application services, task DTOs, and GUI-facing workflows |
| `frontend/` | Vanilla TypeScript GUI and live task/event rendering |
| `internal/nfc/` | Reader abstraction, mock reader, and libnfc boundary |
| `internal/workflow/` | Read, write, dump, restore, key and recovery workflows |
| `internal/attack/` | Controlled external-engine interfaces and process handling |
| `tools/virtual_lab/` | Offline synthetic-card audit and clone-detection lab |
| `docs/` | Architecture, specifications, implementation notes, and release material |
| `testdata/` | Sanitized fixtures for non-hardware tests |

## GUI workflow

The main window exposes reader state, card facts, sector/block workbench data, key status, task progress, activity logs, and guarded write actions. The **Virtual Card Lab** panel is separate from the real-reader path. It provides:

- **One-click bounded audit**: demonstrates weak-secret auditing against the synthetic card only;
- **One-click clone detection**: copies a static synthetic snapshot in memory and demonstrates why dynamic authentication and counters reject it.

The same task bar shows stage progress and the cancel action for both real authorized workflows and the virtual lab.

## Build and validation

The repository is a Wails v2 / Go / Vanilla TypeScript project. A normal development environment should provide Go 1.25.1, Wails v2, Node.js, the frontend dependencies, and the platform-specific libnfc toolchain.

```bash
# virtual lab, no hardware required
python3 -m unittest -v tools/virtual_lab/test_virtual_card_lab.py
python3 tools/virtual_lab/virtual_card_lab.py --demo

# full project, in an environment with Go/Wails installed
wails generate module
go test ./...
cd frontend && npm run build
```

Real-hardware tests must be explicitly enabled and must use designated test cards and test systems. They must never run as part of an ordinary offline test command.

## Data and privacy

Card UIDs, keys, dumps, and card contents are not uploaded by the virtual lab. Review telemetry settings before enabling optional anonymous usage events. Do not commit real card data, production keys, or customer information to this public repository.

## License and third-party notices

NNN source code is licensed under the MIT License. Runtime components and external tools may have separate licenses. Read [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before redistribution.

## Responsible use

NNN is a security-testing workbench, not an authorization bypass tool. Test only within a written scope, use isolated fixtures or designated test cards, preserve evidence without exposing secrets, and stop when a test exceeds the approved scope.

- [Chinese README](README.zh-CN.md)
- [Documentation index](docs/README.md)
- [Public repository](https://github.com/lanruowujie/NNN)
