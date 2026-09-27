import serial
import time

PORT = "/dev/cu.usbserial-A50285BI"
BAUD = 115200

KEY_A = bytes.fromhex("FF FF FF FF FF FF")
WAKE = bytes.fromhex("55 55 00 00 00")

# ------------------------------------------------------------
# PN532 framing
# ------------------------------------------------------------

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
    """
    Extract PN532 -> Host payload beginning with D5.
    Returns None if no complete response frame is found.
    """
    for i in range(max(0, len(raw) - 5)):
        if raw[i:i+3] != b"\x00\x00\xff":
            continue

        if i + 5 > len(raw):
            continue

        length = raw[i+3]

        # ACK frame
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


def send_command(ser, payload, wait=0.001):
    """
    Uses the wake+frame method already proven to work
    with this PN532/FT232 setup.

    Returns:
        raw, parsed_payload
    """
    frame = build_frame(payload)

    ser.reset_input_buffer()

    ser.write(WAKE + frame)
    ser.flush()

    time.sleep(wait)

    raw = ser.read(256)
    parsed = extract_response(raw)

    return raw, parsed


def rawstr(raw):
    if not raw:
        return "<NO SERIAL DATA>"
    return raw.hex(" ").upper()


# ------------------------------------------------------------
# Card discovery
# ------------------------------------------------------------

def scan_card(ser):
    raw, p = send_command(
        ser,
        bytes([0xD4, 0x4A, 0x01, 0x00]),
        wait=0.001
    )

    if not raw:
        return {
            "status": "TIMEOUT",
            "raw": raw
        }

    if not p:
        return {
            "status": "BAD_RESPONSE",
            "raw": raw
        }

    if len(p) < 3 or p[1] != 0x4B:
        return {
            "status": "BAD_RESPONSE",
            "raw": raw
        }

    if p[2] == 0:
        return {
            "status": "NO_CARD",
            "raw": raw
        }

    if len(p) < 8:
        return {
            "status": "BAD_RESPONSE",
            "raw": raw
        }

    target = p[3]
    atqa = p[4:6]
    sak = p[6]
    uid_len = p[7]
    uid = p[8:8 + uid_len]

    if len(uid) != uid_len:
        return {
            "status": "BAD_RESPONSE",
            "raw": raw
        }

    return {
        "status": "OK",
        "target": target,
        "atqa": atqa,
        "sak": sak,
        "uid": uid,
        "raw": raw
    }


# ------------------------------------------------------------
# MIFARE Classic
# ------------------------------------------------------------

def authenticate(ser, target, block, uid):
    """
    Authenticate using Key A = FF FF FF FF FF FF

    MIFARE command 0x60 = Authenticate Key A
    """

    cmd = (
        bytes([
            0xD4,
            0x40,       # InDataExchange
            target,
            0x60,       # Authenticate Key A
            block
        ])
        + KEY_A
        + uid[-4:]
    )

    raw, p = send_command(ser, cmd)

    if not raw:
        return "TIMEOUT", None, raw

    if not p:
        return "BAD_RESPONSE", None, raw

    if len(p) < 3 or p[1] != 0x41:
        return "BAD_RESPONSE", None, raw

    status = p[2]

    if status == 0x00:
        return "OK", status, raw

    return "DENIED", status, raw


def read_block(ser, target, block):
    """
    MIFARE Classic READ command = 0x30
    """

    cmd = bytes([
        0xD4,
        0x40,
        target,
        0x30,
        block
    ])

    raw, p = send_command(ser, cmd)

    if not raw:
        return "TIMEOUT", None, None, raw

    if not p:
        return "BAD_RESPONSE", None, None, raw

    if len(p) < 3 or p[1] != 0x41:
        return "BAD_RESPONSE", None, None, raw

    status = p[2]

    if status != 0x00:
        return "DENIED", status, None, raw

    if len(p) < 19:
        return "SHORT_RESPONSE", status, None, raw

    return "OK", status, p[3:19], raw


# ------------------------------------------------------------
# Main
# ------------------------------------------------------------

ser = serial.Serial(
    PORT,
    BAUD,
    timeout=0.8
)

time.sleep(0.3)

print("=== PN532 MIFARE Classic Diagnostic Dumper ===")
print("READ-ONLY: no write commands will be sent.")
print()


# ------------------------------------------------------------
# Firmware test
# ------------------------------------------------------------

print("[1] Checking PN532...")

raw, p = send_command(
    ser,
    bytes([0xD4, 0x02])
)

if not p:
    print("PN532 TIMEOUT / BAD CONNECTION")
    print("RAW:", rawstr(raw))
    ser.close()
    raise SystemExit

print("PN532 communication: OK")


# ------------------------------------------------------------
# SAM configuration
# ------------------------------------------------------------

print("[2] SAMConfiguration...")

raw, p = send_command(
    ser,
    bytes([
        0xD4,
        0x14,
        0x01,
        0x14,
        0x01
    ])
)

if not p:
    print("SAMConfiguration: TIMEOUT")
    print("RAW:", rawstr(raw))
    ser.close()
    raise SystemExit

print("SAMConfiguration: OK")


# ------------------------------------------------------------
# Scan
# ------------------------------------------------------------

print("[3] Scanning card...")

card = scan_card(ser)

if card["status"] != "OK":
    print("CARD SCAN:", card["status"])
    print("RAW:", rawstr(card.get("raw", b"")))
    ser.close()
    raise SystemExit


target = card["target"]
uid = card["uid"]

print()
print("Card detected")
print("UID :", uid.hex(" ").upper())
print("ATQA:", card["atqa"].hex(" ").upper())
print(f"SAK : {card['sak']:02X}")
print()


# ------------------------------------------------------------
# Dump sectors
# ------------------------------------------------------------

for sector in range(16):

    first_block = sector * 4

    print(f"========== Sector {sector:02d} ==========")

    # Authenticate sector
    result, status, raw = authenticate(
        ser,
        target,
        first_block,
        uid
    )

    if result == "TIMEOUT":
        print("AUTH: PN532 TIMEOUT")
        print("      ^ likely UART/contact problem")
        print("RAW :", rawstr(raw))
        print()
        continue

    if result == "BAD_RESPONSE":
        print("AUTH: BAD/INCOMPLETE PN532 RESPONSE")
        print("RAW :", rawstr(raw))
        print()
        continue

    if result == "DENIED":
        print(
            f"AUTH: DENIED "
            f"(PN532 status=0x{status:02X})"
        )
        print(
            "      Key A FF FF FF FF FF FF "
            "was not accepted."
        )
        print("RAW :", rawstr(raw))
        print()
        continue

    print("AUTH: OK")

    # Read four blocks
    for offset in range(4):

        block = first_block + offset

        result, status, data, raw = read_block(
            ser,
            target,
            block
        )

        prefix = f"Block {block:02d}:"

        if result == "OK":

            trailer = ""

            if offset == 3:
                trailer = "  [SECTOR TRAILER]"

            print(
                prefix,
                data.hex(" ").upper(),
                trailer
            )

        elif result == "TIMEOUT":

            print(
                prefix,
                "<PN532 TIMEOUT>"
            )

        elif result == "DENIED":

            print(
                prefix,
                f"<READ DENIED status=0x{status:02X}>"
            )

        elif result == "SHORT_RESPONSE":

            print(
                prefix,
                "<SHORT RESPONSE>"
            )
            print(
                "          RAW:",
                rawstr(raw)
            )

        else:

            print(
                prefix,
                "<BAD RESPONSE>"
            )
            print(
                "          RAW:",
                rawstr(raw)
            )

    print()


ser.close()

print("=== DONE ===")