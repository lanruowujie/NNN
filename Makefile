.PHONY: fmt test vet frontend-build check dev build release-manifest external-engine-fixture toolchain-check libnfc-build libnfc-verify libnfc-smoke libnfc-repeatability-check libnfc-hardware-smoke libnfc-binding-test libnfc-binding-hardware-smoke libnfc-classic-hardware-test libnfc-dump-hardware-test libnfc-dump-restore-hardware-test mfoc-check mfoc-build mfoc-verify mfcuk-check mfcuk-build mfcuk-verify hardnested-check hardnested-build hardnested-verify

LIBNFC_PLATFORM ?= $(shell go env GOOS)-$(shell go env GOARCH)
LIBNFC_SDK_DIR ?= $(CURDIR)/build/toolchain/$(LIBNFC_PLATFORM)/sdk
LIBNFC_RUNTIME_DIR ?= $(CURDIR)/runtime/$(LIBNFC_PLATFORM)
VERSION ?= 0.1.0
COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= unknown
DIRTY ?= $(shell test -z "$$(git status --porcelain 2>/dev/null)" && echo false || echo true)
RELEASE_LDFLAGS = -X github.com/BennyThink/NFCX/internal/buildinfo.Version=$(VERSION) -X github.com/BennyThink/NFCX/internal/buildinfo.Commit=$(COMMIT) -X github.com/BennyThink/NFCX/internal/buildinfo.BuildDate=$(BUILD_DATE) -X github.com/BennyThink/NFCX/internal/buildinfo.Dirty=$(DIRTY)
NFCX_NATIVE_TAGS := libnfc
ifneq (,$(filter linux-%,$(LIBNFC_PLATFORM)))
NFCX_NATIVE_TAGS := libnfc,webkit2_41
endif

fmt:
	gofmt -w app cmd internal

test:
	go test ./...

vet:
	go vet ./...

frontend-build:
	wails generate module
	cd frontend && npm run build

check: test vet frontend-build

dev:
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	env PATH="$(LIBNFC_RUNTIME_DIR):$$PATH" PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 wails dev -forcebuild -tags "$(NFCX_NATIVE_TAGS)"

build:
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	@test -x "$(LIBNFC_RUNTIME_DIR)/mfoc$(if $(filter windows-%,$(LIBNFC_PLATFORM)),.exe,)" || (echo "MFOC runtime is missing; run make mfoc-build first" >&2; exit 2)
	@test -x "$(LIBNFC_RUNTIME_DIR)/mfcuk$(if $(filter windows-%,$(LIBNFC_PLATFORM)),.exe,)" || (echo "MFCUK runtime is missing; run make mfcuk-build first" >&2; exit 2)
	@test -x "$(LIBNFC_RUNTIME_DIR)/mfoc-hardnested$(if $(filter windows-%,$(LIBNFC_PLATFORM)),.exe,)" || (echo "MFOC-Hardnested runtime is missing; run make hardnested-build first" >&2; exit 2)
	@test -x "$(LIBNFC_RUNTIME_DIR)/nfc-mfsetuid$(if $(filter windows-%,$(LIBNFC_PLATFORM)),.exe,)" || (echo "nfc-mfsetuid runtime is missing; run make libnfc-build first" >&2; exit 2)
	env PATH="$(LIBNFC_RUNTIME_DIR):$$PATH" PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 wails build -trimpath -tags "$(NFCX_NATIVE_TAGS)" -ldflags "$(RELEASE_LDFLAGS)"
	./scripts/package-runtime.sh

release-manifest:
	./scripts/release/generate-manifest.sh "$(VERSION)" "$(COMMIT)"

# Development/acceptance fixture only. Release packaging must not include it.
external-engine-fixture:
	mkdir -p "$(LIBNFC_RUNTIME_DIR)"
	go build -o "$(LIBNFC_RUNTIME_DIR)/nfcx-fake-engine$(if $(filter windows-%,$(LIBNFC_PLATFORM)),.exe,)" ./cmd/nfcx-fake-engine

mfoc-check:
	./scripts/toolchain/mfoc.sh check

mfoc-build:
	./scripts/toolchain/mfoc.sh build

mfoc-verify:
	./scripts/toolchain/mfoc.sh verify

mfcuk-check:
	./scripts/toolchain/mfcuk.sh check

mfcuk-build:
	./scripts/toolchain/mfcuk.sh build

mfcuk-verify:
	./scripts/toolchain/mfcuk.sh verify

hardnested-check:
	./scripts/toolchain/hardnested.sh check

hardnested-build:
	./scripts/toolchain/hardnested.sh build

hardnested-verify:
	./scripts/toolchain/hardnested.sh verify

toolchain-check:
	./scripts/toolchain/libnfc.sh check
	./scripts/toolchain/mfoc.sh check
	./scripts/toolchain/mfcuk.sh check
	./scripts/toolchain/hardnested.sh check

libnfc-build:
	./scripts/toolchain/libnfc.sh build

libnfc-verify:
	./scripts/toolchain/libnfc.sh verify

libnfc-smoke:
	./scripts/toolchain/libnfc.sh smoke

libnfc-repeatability-check:
	./scripts/toolchain/libnfc.sh repeatability-check

libnfc-hardware-smoke:
	@test -n "$(LIBNFC_DEVICE)" || (echo "LIBNFC_DEVICE is required, for example pn532_uart:/dev/cu.usbserial-..." >&2; exit 2)
	./scripts/toolchain/libnfc.sh hardware-smoke "$(LIBNFC_DEVICE)"

libnfc-binding-test:
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	@test -d "$(LIBNFC_RUNTIME_DIR)" || (echo "libnfc runtime is missing; run make libnfc-build first" >&2; exit 2)
	env PATH="$(LIBNFC_RUNTIME_DIR):$$PATH" PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 go test -tags "$(NFCX_NATIVE_TAGS)" ./...

libnfc-binding-hardware-smoke:
	@test -n "$(LIBNFC_DEVICE)" || (echo "LIBNFC_DEVICE is required, for example pn532_uart:/dev/cu.usbserial-..." >&2; exit 2)
	@case "$(LIBNFC_DEVICE)" in pn532_uart:*) ;; *) echo "only a pn532_uart connstring is accepted in this stage" >&2; exit 2 ;; esac
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	env PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 LIBNFC_DEVICE="$(LIBNFC_DEVICE)" LIBNFC_AUTO_SCAN=false LIBNFC_INTRUSIVE_SCAN=false go test -count=1 -timeout=10m -tags "libnfc libnfc_hardware" ./internal/nfc/libnfc

libnfc-classic-hardware-test:
	@test -n "$(LIBNFC_DEVICE)" || (echo "LIBNFC_DEVICE is required, for example pn532_uart:/dev/cu.usbserial-..." >&2; exit 2)
	@case "$(LIBNFC_DEVICE)" in pn532_uart:*) ;; *) echo "only a pn532_uart connstring is accepted in this stage" >&2; exit 2 ;; esac
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	env PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 LIBNFC_DEVICE="$(LIBNFC_DEVICE)" LIBNFC_AUTO_SCAN=false LIBNFC_INTRUSIVE_SCAN=false go test -count=1 -timeout=2m -run '^TestHardwareMIFAREClassicKnownKeyIO$$' -tags "libnfc libnfc_hardware" ./internal/nfc/libnfc

libnfc-dump-hardware-test:
	@test -n "$(LIBNFC_DEVICE)" || (echo "LIBNFC_DEVICE is required, for example pn532_uart:/dev/cu.usbserial-..." >&2; exit 2)
	@case "$(LIBNFC_DEVICE)" in pn532_uart:*) ;; *) echo "only a pn532_uart connstring is accepted in this stage" >&2; exit 2 ;; esac
	@test -n "$(NFCX_TEST_DUMP_PATH)" || (echo "NFCX_TEST_DUMP_PATH is required so the dump is preserved" >&2; exit 2)
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	env PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 LIBNFC_DEVICE="$(LIBNFC_DEVICE)" LIBNFC_AUTO_SCAN=false LIBNFC_INTRUSIVE_SCAN=false NFCX_WORKFLOW_HARDWARE=1 NFCX_TEST_DUMP_PATH="$(NFCX_TEST_DUMP_PATH)" NFCX_TEST_KEY_A="$(NFCX_TEST_KEY_A)" NFCX_TEST_KEY_B="$(NFCX_TEST_KEY_B)" go test -v -count=1 -timeout=10m -run '^TestHardwareMIFAREClassicDumpRestore$$' -tags "libnfc libnfc_hardware" ./internal/workflow

libnfc-dump-restore-hardware-test:
	@test -n "$(LIBNFC_DEVICE)" || (echo "LIBNFC_DEVICE is required, for example pn532_uart:/dev/cu.usbserial-..." >&2; exit 2)
	@case "$(LIBNFC_DEVICE)" in pn532_uart:*) ;; *) echo "only a pn532_uart connstring is accepted in this stage" >&2; exit 2 ;; esac
	@test "$(NFCX_HARDWARE_RESTORE_CONFIRM)" = "I_OWN_THIS_CARD" || (echo "set NFCX_HARDWARE_RESTORE_CONFIRM=I_OWN_THIS_CARD to authorize the full-card restore test" >&2; exit 2)
	@test -n "$(NFCX_TEST_DUMP_PATH)" || (echo "NFCX_TEST_DUMP_PATH is required so the recovery dump is preserved" >&2; exit 2)
	@test -f "$(LIBNFC_SDK_DIR)/lib/pkgconfig/libnfc.pc" || (echo "libnfc SDK is missing; run make libnfc-build first" >&2; exit 2)
	env PKG_CONFIG_PATH="$(LIBNFC_SDK_DIR)/lib/pkgconfig" DYLD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" LD_LIBRARY_PATH="$(LIBNFC_RUNTIME_DIR)" CGO_ENABLED=1 LIBNFC_DEVICE="$(LIBNFC_DEVICE)" LIBNFC_AUTO_SCAN=false LIBNFC_INTRUSIVE_SCAN=false NFCX_WORKFLOW_HARDWARE=1 NFCX_HARDWARE_RESTORE_CONFIRM="$(NFCX_HARDWARE_RESTORE_CONFIRM)" NFCX_TEST_DUMP_PATH="$(NFCX_TEST_DUMP_PATH)" NFCX_TEST_KEY_A="$(NFCX_TEST_KEY_A)" NFCX_TEST_KEY_B="$(NFCX_TEST_KEY_B)" go test -count=1 -timeout=10m -run '^TestHardwareMIFAREClassicDumpRestore$$' -tags "libnfc libnfc_hardware" ./internal/workflow
