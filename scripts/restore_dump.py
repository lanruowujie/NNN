#!/usr/bin/env python3
"""Restore a raw 1 KiB MIFARE Classic dump through a PN532 UART reader.

Only use this with cards and systems you own or are authorized to test.
Block 0 is skipped unless --write-block0 is explicitly supplied, because it is
manufacturer-programmed on normal cards and writable only on compatible magic
/ CUID cards.
"""

import argparse
import sys
import time
from pathlib import Path

import serial


DEFAULT_DUMP = "mifare_469C8A12_20260911_201131.bin"
DEFAULT_PORT = "/dev/cu.usbserial-A50285BI"
DEFAULT_KEY_A = bytes.fromhex("FF FF FF FF FF FF")
WAKE = bytes.fromhex("55 55 00 00 00")
COMMAND_TIMEOUT = 0.30
SCAN_TIMEOUT = 0.80


def build_frame(payload: bytes) -> bytes:
    length = len(payload)
    return (
        bytes([0x00, 0x00, 0xFF, length, (-length) & 0xFF])
        + payload
        + bytes([(-sum(payload)) & 0xFF, 0x00])
    )


def extract_response(raw: bytes):
    i = 0
    while i <= len(raw) - 6:
        if raw[i:i + 3] != b"\x00\x00\xff":
            i += 1
            continue

        length = raw[i + 3]
        lcs = raw[i + 4]
        if length == 0 and lcs == 0xFF:  # ACK
            i += 6
            continue

        frame_end = i + 5 + length + 2
        if frame_end > len(raw):
            return None

        payload = raw[i + 5:i + 5 + length]
        if payload and payload[0] == 0xD5:
            return payload
        i = frame_end
    return None


def send_command(ser, payload: bytes, timeout=COMMAND_TIMEOUT):
    ser.reset_input_buffer()
    ser.write(WAKE + build_frame(payload))
    ser.flush()

    deadline = time.monotonic() + timeout
    raw = bytearray()
    while time.monotonic() < deadline:
        if ser.in_waiting:
            raw.extend(ser.read(ser.in_waiting))
            response = extract_response(bytes(raw))
            if response is not None:
                return bytes(raw), response
        time.sleep(0.0005)
    return bytes(raw), extract_response(bytes(raw))


def sam_configuration(ser) -> bool:
    _, response = send_command(
        ser,
        bytes([0xD4, 0x14, 0x01, 0x14, 0x01]),
        timeout=0.50,
    )
    return bool(response and len(response) >= 2 and response[1] == 0x15)


def scan_card(ser):
    _, response = send_command(
        ser,
        bytes([0xD4, 0x4A, 0x01, 0x00]),
        timeout=SCAN_TIMEOUT,
    )
    if not response or len(response) < 8 or response[1] != 0x4B or response[2] < 1:
        return None

    uid_length = response[7]
    uid = response[8:8 + uid_length]
    if len(uid) != uid_length:
        return None
    return {
        "target": response[3],
        "atqa": response[4:6],
        "sak": response[6],
        "uid": uid,
    }


def authenticate(ser, target: int, block: int, uid: bytes, key: bytes) -> bool:
    payload = bytes([0xD4, 0x40, target, 0x60, block]) + key + uid[-4:]
    _, response = send_command(ser, payload)
    return bool(
        response
        and len(response) >= 3
        and response[1] == 0x41
        and response[2] == 0x00
    )


def read_block(ser, target: int, block: int):
    _, response = send_command(ser, bytes([0xD4, 0x40, target, 0x30, block]))
    if (
        not response
        or len(response) < 19
        or response[1] != 0x41
        or response[2] != 0x00
    ):
        return None
    return response[3:19]


def write_block(ser, target: int, block: int, data: bytes) -> bool:
    if len(data) != 16:
        raise ValueError("A MIFARE Classic block must contain exactly 16 bytes")
    payload = bytes([0xD4, 0x40, target, 0xA0, block]) + data
    _, response = send_command(ser, payload, timeout=0.60)
    return bool(
        response
        and len(response) >= 3
        and response[1] == 0x41
        and response[2] == 0x00
    )


def access_bits_are_valid(trailer: bytes) -> bool:
    b6, b7, b8 = trailer[6:9]
    return (
        ((b6 & 0x0F) ^ ((b7 >> 4) & 0x0F)) == 0x0F
        and (((b6 >> 4) & 0x0F) ^ (b8 & 0x0F)) == 0x0F
        and ((b7 & 0x0F) ^ ((b8 >> 4) & 0x0F)) == 0x0F
    )


def load_dump(path: Path, source_key_a: bytes):
    image = bytearray(path.read_bytes())
    if len(image) != 1024:
        raise ValueError(f"镜像必须正好为 1024 字节，当前为 {len(image)} 字节")

    expected_bcc = image[0] ^ image[1] ^ image[2] ^ image[3]
    if image[4] != expected_bcc:
        raise ValueError(
            f"Block 0 BCC 无效：文件中为 {image[4]:02X}，应为 {expected_bcc:02X}"
        )

    for sector in range(16):
        trailer_offset = (sector * 4 + 3) * 16
        trailer = image[trailer_offset:trailer_offset + 16]
        if not access_bits_are_valid(trailer):
            raise ValueError(f"Sector {sector} 的访问控制位无效，拒绝写入")

        # Key A is never returned by a normal MIFARE Classic READ.  The dumper
        # therefore stored six zero bytes here; restore the key that was used
        # successfully while making the dump.
        image[trailer_offset:trailer_offset + 6] = source_key_a

    return bytes(image)


def verify_written_block(ser, target: int, block: int, expected: bytes) -> bool:
    actual = read_block(ser, target, block)
    if actual is None:
        return False
    if block % 4 == 3:
        # Reading a sector trailer masks Key A, so only bytes 6..15 can be
        # compared.  Key A is proven separately by authentication.
        return actual[6:] == expected[6:]
    return actual == expected


def parse_key(value: str) -> bytes:
    try:
        key = bytes.fromhex(value.replace(":", "").replace(" ", ""))
    except ValueError as exc:
        raise argparse.ArgumentTypeError("密钥必须是 12 个十六进制字符") from exc
    if len(key) != 6:
        raise argparse.ArgumentTypeError("密钥必须是 6 字节")
    return key


def parse_args():
    parser = argparse.ArgumentParser(description="将 1K MIFARE Classic BIN 镜像写入目标卡")
    parser.add_argument("dump", nargs="?", default=DEFAULT_DUMP, type=Path)
    parser.add_argument("--port", default=DEFAULT_PORT)
    parser.add_argument(
        "--source-key-a",
        type=parse_key,
        default=DEFAULT_KEY_A,
        help="制作镜像时使用的 Key A（默认 FFFFFFFFFFFF）",
    )
    parser.add_argument(
        "--target-key-a",
        type=parse_key,
        default=DEFAULT_KEY_A,
        help="目标卡写入前的 Key A（默认 FFFFFFFFFFFF）",
    )
    parser.add_argument(
        "--write-block0",
        action="store_true",
        help="最后写入厂商块；仅适用于支持普通写 Block 0 的 CUID/Gen2 卡",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        image = load_dump(args.dump, args.source_key_a)
    except (OSError, ValueError) as exc:
        print(f"ERROR: 无法使用镜像：{exc}", file=sys.stderr)
        return 1

    source_uid = image[:4]
    print(f"镜像: {args.dump.resolve()} (1024 bytes)")
    print(f"源 UID: {source_uid.hex(' ').upper()}")
    print("Block 0:", "将写入" if args.write_block0 else "默认跳过")

    try:
        ser = serial.Serial(args.port, 115200, timeout=0)
    except serial.SerialException as exc:
        print(f"ERROR: 无法打开串口 {args.port}: {exc}", file=sys.stderr)
        return 1

    try:
        time.sleep(0.15)
        if not sam_configuration(ser):
            print("ERROR: PN532 SAMConfiguration 失败", file=sys.stderr)
            return 1

        print("请只放置一张目标卡……")
        card = scan_card(ser)
        if not card:
            print("ERROR: 未检测到卡", file=sys.stderr)
            return 1
        if len(card["uid"]) != 4:
            print("ERROR: 目标卡不是 4-byte UID 卡", file=sys.stderr)
            return 1
        if card["sak"] != 0x08:
            print(
                f"ERROR: 目标卡 SAK={card['sak']:02X}，不是预期的 MIFARE Classic 1K (08)",
                file=sys.stderr,
            )
            return 1

        target_uid = card["uid"]
        print(f"目标 UID: {target_uid.hex(' ').upper()}")
        print(f"ATQA: {card['atqa'].hex(' ').upper()}  SAK: {card['sak']:02X}")
        print("警告：继续操作将覆盖目标卡的数据和密钥，原内容无法自动恢复。")
        # confirmation = input(
        #     f"请输入 Y 确认: "
        # ).strip()
        # if confirmation != f"Y":
        #     print("已取消，未写入任何数据。")
        #     return 0

        target = card["target"]
        written = 0
        for sector in range(16):
            first_block = sector * 4
            if not authenticate(
                ser, target, first_block, target_uid, args.target_key_a
            ):
                print(f"ERROR: Sector {sector} 使用目标 Key A 认证失败", file=sys.stderr)
                return 1

            blocks = list(range(first_block, first_block + 4))
            if sector == 0:
                blocks.remove(0)

            # The sector trailer is already last in this order.  This matters:
            # writing it can change the key and access conditions.
            for block in blocks:
                expected = image[block * 16:(block + 1) * 16]
                if not write_block(ser, target, block, expected):
                    print(f"ERROR: Block {block} 写入失败", file=sys.stderr)
                    return 1
                if block % 4 == 3 and not authenticate(
                    ser, target, block, target_uid, args.source_key_a
                ):
                    print(f"ERROR: Sector {sector} 新 Key A 校验失败", file=sys.stderr)
                    return 1
                if not verify_written_block(ser, target, block, expected):
                    print(f"ERROR: Block {block} 写后校验失败", file=sys.stderr)
                    return 1
                written += 1
                print(f"Block {block:02d}: OK")

        if args.write_block0:
            if not authenticate(ser, target, 0, target_uid, args.source_key_a):
                print("ERROR: 写 Block 0 前重新认证 Sector 0 失败", file=sys.stderr)
                return 1
            if not write_block(ser, target, 0, image[:16]):
                print(
                    "ERROR: Block 0 写入失败；目标卡可能不是可普通写 UID 的 CUID/Gen2 卡",
                    file=sys.stderr,
                )
                return 1
            written += 1
            time.sleep(0.20)
            card_after = None
            for _ in range(3):
                card_after = scan_card(ser)
                if card_after:
                    break
                time.sleep(0.15)
            if not card_after or card_after["uid"] != source_uid:
                print("ERROR: Block 0 写入后 UID 校验失败", file=sys.stderr)
                return 1
            if not authenticate(
                ser,
                card_after["target"],
                0,
                card_after["uid"],
                args.source_key_a,
            ):
                print("ERROR: Block 0 写入后重新认证失败", file=sys.stderr)
                return 1
            if read_block(ser, card_after["target"], 0) != image[:16]:
                print("ERROR: Block 0 写后内容校验失败", file=sys.stderr)
                return 1
            print("Block 00: OK（UID 已校验）")

        print(f"完成：已写入并校验 {written} 个块。")
        if not args.write_block0:
            print("目标卡保留自身 UID；如需完整复制且目标是 CUID/Gen2，请加 --write-block0。")
        return 0
    finally:
        ser.close()


if __name__ == "__main__":
    raise SystemExit(main())
