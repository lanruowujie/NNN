#!/usr/bin/env python3
# coding: utf-8

# mifare - cuid.py
import serial
import time

PORT = "/dev/cu.usbserial-A50285BI"
BAUD = 115200

KEY_A = bytes.fromhex("FF FF FF FF FF FF")
# NEW_UID = bytes.fromhex("46 9C 8A 12")
NEW_UID = bytes.fromhex("12 34 56 78")
#311073862
WAKE = bytes.fromhex("55 55 00 00 00")


def build_frame(payload: bytes) -> bytes:
    length = len(payload)
    lcs = (-length) & 0xFF
    dcs = (-sum(payload)) & 0xFF

    return (
        bytes([0x00, 0x00, 0xFF, length, lcs])
        + payload
        + bytes([dcs, 0x00])
    )


def extract_response(raw: bytes):
    for i in range(max(0, len(raw) - 5)):
        if raw[i:i+3] != b"\x00\x00\xff":
            continue

        if i + 5 > len(raw):
            continue

        length = raw[i+3]

        # ACK
        if length == 0:
            continue

        start = i + 5
        end = start + length

        if end > len(raw):
            continue

        payload = raw[start:end]

        if payload and payload[0] == 0xD5:
            return payload

    return None


def send_command(ser, payload, wait=0.25):
    frame = build_frame(payload)

    ser.reset_input_buffer()
    ser.write(WAKE + frame)
    ser.flush()

    time.sleep(wait)

    raw = ser.read(256)
    return raw, extract_response(raw)


def scan_card(ser):
    raw, p = send_command(
        ser,
        bytes([0xD4, 0x4A, 0x01, 0x00]),
        wait=0.1
    )

    if not p or len(p) < 8:
        return None

    if p[1] != 0x4B or p[2] < 1:
        return None

    target = p[3]
    atqa = p[4:6]
    sak = p[6]
    uid_len = p[7]
    uid = p[8:8 + uid_len]

    return target, atqa, sak, uid


def authenticate(ser, target, block, uid):
    payload = (
        bytes([
            0xD4,
            0x40,
            target,
            0x60,       # Key A authentication
            block
        ])
        + KEY_A
        + uid[-4:]
    )

    raw, p = send_command(ser, payload)

    if not p or len(p) < 3:
        return False

    return p[1] == 0x41 and p[2] == 0x00


def read_block(ser, target, block):
    payload = bytes([
        0xD4,
        0x40,
        target,
        0x30,           # MIFARE READ
        block
    ])

    raw, p = send_command(ser, payload)

    if not p or len(p) < 19:
        return None

    if p[1] != 0x41 or p[2] != 0x00:
        return None

    return p[3:19]


def write_block(ser, target, block, data):
    assert len(data) == 16

    payload = (
        bytes([
            0xD4,
            0x40,
            target,
            0xA0,       # MIFARE WRITE
            block
        ])
        + data
    )

    raw, p = send_command(ser, payload, wait=0.5)

    print("WRITE RAW:", raw.hex(" ").upper())

    if not p or len(p) < 3:
        return False

    return p[1] == 0x41 and p[2] == 0x00


# --------------------------------------------------

if len(NEW_UID) != 4:
    raise ValueError("This script only supports 4-byte UID cards.")

ser = serial.Serial(PORT, BAUD, timeout=1)
time.sleep(0.3)

print("=== PN532 CUID UID Test ===")
print("WARNING: This WILL WRITE Block 0.")
print()

# SAMConfiguration
raw, p = send_command(
    ser,
    bytes([0xD4, 0x14, 0x01, 0x14, 0x01])
)

if not p:
    print("SAMConfiguration failed.")
    ser.close()
    raise SystemExit


print("Put ONE test CUID card on the PN532...")
card = scan_card(ser)

if not card:
    print("No card detected.")
    ser.close()
    raise SystemExit


target, atqa, sak, old_uid = card

print()
print("Current card:")
print("UID :", old_uid.hex(" ").upper())
print("ATQA:", atqa.hex(" ").upper())
print(f"SAK : {sak:02X}")


if len(old_uid) != 4:
    print()
    print("STOP: card does not have a 4-byte UID.")
    ser.close()
    raise SystemExit


# Authenticate sector 0
if not authenticate(
    ser,
    target,
    0,
    old_uid
):
    print()
    print("Authentication failed.")
    print("Default Key A FFFFFFFFFFFF was not accepted.")
    ser.close()
    raise SystemExit


# Read original Block 0
old_block0 = read_block(
    ser,
    target,
    0
)

if old_block0 is None:
    print("Could not read Block 0.")
    ser.close()
    raise SystemExit


print()
print("Original Block 0:")
print(old_block0.hex(" ").upper())


# Calculate BCC
bcc = (
    NEW_UID[0]
    ^ NEW_UID[1]
    ^ NEW_UID[2]
    ^ NEW_UID[3]
)

# Keep manufacturer bytes 5..15 unchanged
new_block0 = (
    NEW_UID
    + bytes([bcc])
    + old_block0[5:]
)


print()
print("Proposed Block 0:")
print(new_block0.hex(" ").upper())

print()
print("UID:")
print(
    old_uid.hex(" ").upper(),
    " -> ",
    NEW_UID.hex(" ").upper()
)

print(f"BCC: {bcc:02X}")

print()
print("The remaining 11 bytes will NOT be changed.")
print()


# answer = input(
#     'Type exactly "WRITE" to modify Block 0: '
# )
#
# if answer != "WRITE":
#     print("Cancelled. Nothing written.")
#     ser.close()
#     raise SystemExit


print()
print("Writing Block 0...")

success = write_block(
    ser,
    target,
    0,
    new_block0
)

if not success:
    print()
    print("WRITE FAILED.")
    print()
    print(
        "This may mean the card is not a normal-write CUID/Gen2 card."
    )
    print(
        "Do NOT keep blindly sending write commands."
    )
    ser.close()
    raise SystemExit


print("WRITE command accepted.")

# Give the card/PN532 a moment
time.sleep(0.5)

print()
print("Re-scanning card...")

card2 = scan_card(ser)

if not card2:
    print(
        "Could not re-select card. "
        "Remove it, place it back, and run the reader again."
    )
    ser.close()
    raise SystemExit


_, atqa2, sak2, uid2 = card2

print()
print("=== RESULT ===")
print("Old UID:", old_uid.hex(" ").upper())
print("New UID:", uid2.hex(" ").upper())
print("ATQA   :", atqa2.hex(" ").upper())
print(f"SAK    : {sak2:02X}")

if uid2 == NEW_UID:
    print()
    print("SUCCESS: UID changed correctly.")
else:
    print()
    print("UID did not change to the requested value.")


ser.close()