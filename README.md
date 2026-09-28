# NNN

NNN is a cross-platform GUI workbench for NFC/card read-write security testing, protocol validation, data-integrity checks, and clone-detection evaluation. Its workflows cover controlled real environments and an offline synthetic-card laboratory; use follows the target system, card ownership, and applicable testing standards.

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

NNN is intended for real-environment testing of:

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

The same task bar shows stage progress and the cancel action for both real-environment workflows and the virtual lab.

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

Real-hardware tests are explicitly separated from ordinary offline test commands.

## Data and privacy

Card UIDs, keys, dumps, and card contents are not uploaded by the virtual lab. Review telemetry settings before enabling optional anonymous usage events. Do not commit real card data, production keys, or customer information to this public repository.

## License and third-party notices

NNN source code is licensed under the MIT License. Runtime components and external tools may have separate licenses. Read [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before redistribution.

## Testing practice

Use isolated fixtures, designated test cards, sanitized evidence, and the testing standard defined for the target system.

- [Chinese README](README.zh-CN.md)
- [Documentation index](docs/README.md)
- [Public repository](https://github.com/lanruowujie/NNN)
