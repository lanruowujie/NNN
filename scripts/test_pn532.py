#!/usr/bin/env python3
# coding: utf-8

# mifare - test_pn532.py

import serial
import time

PORT = "/dev/cu.usbserial-A50285BI"

ser = serial.Serial(PORT, 115200, timeout=1)

# PN532 wake-up preamble + GetFirmwareVersion
cmd = bytes.fromhex(
    "55 55 00 00 00 "
    "00 00 FF 02 FE D4 02 2A 00"
)

ser.write(cmd)
ser.flush()

time.sleep(0.5)

data = ser.read(100)

print("RX:", data.hex(" "))

ser.close()