#!/usr/bin/env python3

import serial
import time
from pathlib import Path
from datetime import datetime


# ============================================================
# Configuration
# ============================================================

PORT = "/dev/cu.usbserial-A50285BI"
BAUD = 115200

# Default MIFARE Classic Key A
KEY_A = bytes.fromhex("FF FF FF FF FF FF")

# This wake sequence has already been verified to work
# with your FT232RL + PN532 setup.
WAKE = bytes.fromhex("55 55 00 00 00")

# Fast timeout for normal MIFARE operations
COMMAND_TIMEOUT = 0.15

# Slightly longer timeout for card discovery
SCAN_TIMEOUT = 0.50


# ============================================================
# PN532 frame helpers
# ============================================================

def build_frame(payload: bytes) -> bytes:
    length = len(payload)
    lcs = (-length) & 0xFF
    dcs = (-sum(payload)) & 0xFF

    return (
        bytes([
            0x00,
            0x00,
            0xFF,
            length,
            lcs
        ])
        + payload
        + bytes([
            dcs,
            0x00
        ])
    )


def extract_response(raw: bytes):
    """
    Search the serial stream for a complete PN532 response.

    ACK frames are ignored.
    """

    i = 0

    while i <= len(raw) - 6:

        # PN532 preamble/start code
        if raw[i:i + 3] != b"\x00\x00\xff":
            i += 1
            continue

        length = raw[i + 3]
        lcs = raw[i + 4]

        # ACK:
        # 00 00 FF 00 FF 00
        if length == 0x00 and lcs == 0xFF:
            i += 6
            continue

        # Normal frame:
        #
        # 00 00 FF LEN LCS
        # DATA...
        # DCS POSTAMBLE

        total_length = 5 + length + 2

        if i + total_length > len(raw):
            return None

        payload_start = i + 5
        payload_end = payload_start + length

        payload = raw[payload_start:payload_end]

        if payload and payload[0] == 0xD5:
            return payload

        i += total_length

    return None


# ============================================================
# Fast serial command
# ============================================================

def send_command(
    ser,
    payload: bytes,
    timeout=COMMAND_TIMEOUT
):
    """
    Send WAKE + PN532 frame together.

    Instead of sleeping a fixed 250 ms,
    continuously check the serial buffer and return
    immediately when the PN532 response is complete.
    """

    frame = build_frame(payload)

    ser.reset_input_buffer()

    # Important:
    # Your setup works reliably when WAKE and frame
    # are sent in ONE write().
    ser.write(WAKE + frame)
    ser.flush()

    deadline = time.monotonic() + timeout
    raw = bytearray()

    while time.monotonic() < deadline:

        waiting = ser.in_waiting

        if waiting:
            raw.extend(
                ser.read(waiting)
            )

            response = extract_response(
                bytes(raw)
            )

            if response is not None:
                return bytes(raw), response

        # Don't burn 100% CPU while polling.
        # 0.5 ms is tiny compared with the old 250 ms delay.
        time.sleep(0.0005)

    return (
        bytes(raw),
        extract_response(bytes(raw))
    )


# ============================================================
# PN532 operations
# ============================================================

def sam_configuration(ser):
    """
    Put PN532 into normal SAM mode.
    """

    raw, p = send_command(
        ser,
        bytes([
            0xD4,
            0x14,
            0x01,
            0x14,
            0x01
        ]),
        timeout=0.30
    )

    if not p:
        return False

    return (
        len(p) >= 2
        and p[0] == 0xD5
        and p[1] == 0x15
    )


def scan_card(ser):
    """
    ISO14443-A passive target discovery.
    """

    raw, p = send_command(
        ser,
        bytes([
            0xD4,
            0x4A,
            0x01,
            0x00
        ]),
        timeout=SCAN_TIMEOUT
    )

    if not p:
        return None

    if len(p) < 8:
        return None

    if p[1] != 0x4B:
        return None

    # Number of targets
    if p[2] < 1:
        return None

    target = p[3]

    atqa = p[4:6]

    sak = p[6]

    uid_len = p[7]

    uid = p[
        8:
        8 + uid_len
    ]

    return (
        target,
        atqa,
        sak,
        uid
    )


def authenticate(
    ser,
    target,
    block,
    uid
):
    """
    MIFARE Classic Authenticate Key A.
    """

    payload = (
        bytes([
            0xD4,
            0x40,
            target,

            # MIFARE Authenticate Key A
            0x60,

            block
        ])
        + KEY_A
        + uid[-4:]
    )

    raw, p = send_command(
        ser,
        payload,
        timeout=COMMAND_TIMEOUT
    )

    if not raw:
        return "TIMEOUT"

    if not p:
        return "BAD_RESPONSE"

    if len(p) < 3:
        return "SHORT_RESPONSE"

    if p[1] != 0x41:
        return "BAD_RESPONSE"

    status = p[2]

    if status != 0x00:
        return f"DENIED(0x{status:02X})"

    return "OK"


def read_block(
    ser,
    target,
    block
):
    """
    Read one 16-byte MIFARE Classic block.
    """

    payload = bytes([
        0xD4,
        0x40,
        target,

        # MIFARE READ
        0x30,

        block
    ])

    raw, p = send_command(
        ser,
        payload,
        timeout=COMMAND_TIMEOUT
    )

    if not raw:
        return None, "TIMEOUT"

    if not p:
        return None, "BAD_RESPONSE"

    if len(p) < 3:
        return None, "SHORT_RESPONSE"

    if p[1] != 0x41:
        return None, "BAD_RESPONSE"

    status = p[2]

    if status != 0x00:
        return (
            None,
            f"DENIED(0x{status:02X})"
        )

    # D5 41 STATUS + 16 bytes
    if len(p) < 19:
        return None, "SHORT_RESPONSE"

    data = p[3:19]

    return data, "OK"


# ============================================================
# Main
# ============================================================

def main():

    print(
        "=== PN532 FAST MIFARE Classic 1K Dumper ==="
    )

    print(
        "READ ONLY - no write commands are used."
    )

    print()

    # Non-blocking serial mode.
    # We handle timeouts ourselves.
    ser = serial.Serial(
        PORT,
        BAUD,
        timeout=0
    )

    # Let FT232/PN532 settle after opening port.
    time.sleep(0.15)

    try:

        # ----------------------------------------------------
        # SAM configuration
        # ----------------------------------------------------

        if not sam_configuration(ser):

            print(
                "ERROR: SAMConfiguration failed."
            )

            return

        # ----------------------------------------------------
        # Find card
        # ----------------------------------------------------

        print(
            "Place ONE card on the PN532..."
        )

        card = scan_card(ser)

        if not card:

            print(
                "ERROR: No card detected."
            )

            return

        (
            target,
            atqa,
            sak,
            uid
        ) = card

        print()

        print("=== CARD ===")

        print(
            "UID :",
            uid.hex(" ").upper()
        )

        print(
            "ATQA:",
            atqa.hex(" ").upper()
        )

        print(
            f"SAK : {sak:02X}"
        )

        print()

        if len(uid) != 4:

            print(
                "WARNING: This script is intended "
                "for 4-byte UID MIFARE Classic cards."
            )

            print()

        # ----------------------------------------------------
        # Dump
        # ----------------------------------------------------

        dump = bytearray()

        failed_blocks = []

        start_time = time.monotonic()

        for sector in range(16):

            first_block = sector * 4

            print(
                f"--- Sector {sector:02d} ---"
            )

            # Authenticate once per sector.
            auth_result = authenticate(
                ser,
                target,
                first_block,
                uid
            )

            print(
                "AUTH:",
                auth_result
            )

            if auth_result != "OK":

                # Preserve binary offsets.
                dump.extend(
                    b"\x00" * 64
                )

                failed_blocks.extend(
                    range(
                        first_block,
                        first_block + 4
                    )
                )

                print()

                continue

            # Read all four blocks in the sector.
            for block in range(
                first_block,
                first_block + 4
            ):

                data, status = read_block(
                    ser,
                    target,
                    block
                )

                trailer = (
                    block % 4 == 3
                )

                if data is None:

                    print(
                        f"Block {block:02d}: "
                        f"<{status}>"
                    )

                    dump.extend(
                        b"\x00" * 16
                    )

                    failed_blocks.append(
                        block
                    )

                    continue

                suffix = ""

                if trailer:
                    suffix = (
                        "  [SECTOR TRAILER]"
                    )

                print(
                    f"Block {block:02d}: "
                    f"{data.hex(' ').upper()}"
                    f"{suffix}"
                )

                dump.extend(data)

            print()

        elapsed = (
            time.monotonic()
            - start_time
        )

        # ----------------------------------------------------
        # Save raw 1K binary dump
        # ----------------------------------------------------

        uid_text = (
            uid.hex().upper()
        )

        timestamp = (
            datetime.now()
            .strftime("%Y%m%d_%H%M%S")
        )

        filename = (
            f"mifare_{uid_text}_"
            f"{timestamp}.bin"
        )

        path = Path(filename)

        path.write_bytes(dump)

        # ----------------------------------------------------
        # Result
        # ----------------------------------------------------

        print("=" * 60)

        print("DONE")

        print()

        print(
            f"Dump time: {elapsed:.3f} seconds"
        )

        print(
            "Dump size:",
            len(dump),
            "bytes"
        )

        print(
            "Saved to:",
            path.resolve()
        )

        print()

        if failed_blocks:

            print(
                "WARNING: Some blocks could "
                "not be read."
            )

            print(
                "Failed blocks:",
                ", ".join(
                    str(x)
                    for x in failed_blocks
                )
            )

            print()

            print(
                "Unread blocks were stored as "
                "16 zero bytes to preserve offsets."
            )

        else:

            print(
                "SUCCESS: All 64 blocks "
                "were read."
            )

        print()

        if len(dump) >= 16:

            print(
                "Block 0:",
                dump[:16]
                .hex(" ")
                .upper()
            )

    finally:

        ser.close()


if __name__ == "__main__":
    main()