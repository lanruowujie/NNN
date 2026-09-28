#!/usr/bin/env python3
"""NNN offline virtual-card security lab.

This module intentionally models a *toy* card protocol. It has no reader,
libnfc, NFC frame handling, real-card support, or authentication bypass.
The audit loop is bounded to a small lab-only candidate set and is intended
to demonstrate why weak secrets and static dumps are unsafe.
"""

from __future__ import annotations

import argparse
import hashlib
import hmac
import json
import secrets
from dataclasses import dataclass, field
from typing import Iterable

MAX_AUDIT_ATTEMPTS = 64
LAB_PROTOCOL = "NNN-VirtualCard/1"


def _derive_secret(password: str, salt: bytes) -> bytes:
    return hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, 20_000, 32)


def _proof(secret: bytes, challenge: bytes) -> bytes:
    return hmac.new(secret, LAB_PROTOCOL.encode() + challenge, hashlib.sha256).digest()[:16]


@dataclass(frozen=True)
class CardSnapshot:
    uid: str
    memory: bytes
    counter: int
    protocol: str = LAB_PROTOCOL

    @property
    def fingerprint(self) -> str:
        return hashlib.sha256(self.uid.encode() + self.memory + self.counter.to_bytes(8, "big")).hexdigest()


@dataclass
class VirtualCard:
    """A synthetic card with static memory and a challenge-response secret."""

    uid: str
    memory: bytearray
    _salt: bytes
    _secret: bytes
    counter: int = 0
    auth_failures: int = 0
    _used_challenges: set[bytes] = field(default_factory=set)

    @classmethod
    def create(cls, password: str = "lab-pass-07", *, uid: str = "LAB-7A31C2", memory: bytes = b"NNN synthetic card") -> "VirtualCard":
        salt = hashlib.sha256((uid + ":salt").encode()).digest()[:16]
        return cls(uid, bytearray(memory), salt, _derive_secret(password, salt))

    def challenge(self) -> bytes:
        challenge = secrets.token_bytes(16)
        self._used_challenges.add(challenge)
        return challenge

    def authenticate(self, password: str, challenge: bytes) -> bool:
        if challenge not in self._used_challenges:
            self.auth_failures += 1
            return False
        self._used_challenges.remove(challenge)
        candidate = _derive_secret(password, self._salt)
        ok = hmac.compare_digest(_proof(candidate, challenge), _proof(self._secret, challenge))
        if ok:
            self.counter += 1
        else:
            self.auth_failures += 1
        return ok

    def snapshot(self) -> CardSnapshot:
        return CardSnapshot(self.uid, bytes(self.memory), self.counter)

    def static_export(self) -> CardSnapshot:
        """Return only dump-like state; the dynamic secret is never exported."""
        return self.snapshot()


@dataclass(frozen=True)
class StaticClone:
    """A deliberate static-dump clone used to test clone detection."""

    snapshot: CardSnapshot

    def authenticate(self, password: str, challenge: bytes) -> bool:
        # A copied dump has no dynamic secret and must not authenticate.
        return False


@dataclass(frozen=True)
class AuditResult:
    found: bool
    candidate: str | None
    attempts: int
    exhausted: bool


def bounded_password_audit(card: VirtualCard, candidates: Iterable[str], *, max_attempts: int = MAX_AUDIT_ATTEMPTS) -> AuditResult:
    """Audit a synthetic card against at most 64 explicitly supplied candidates."""
    if max_attempts < 1 or max_attempts > MAX_AUDIT_ATTEMPTS:
        raise ValueError(f"max_attempts must be between 1 and {MAX_AUDIT_ATTEMPTS}")
    attempts = 0
    for candidate in candidates:
        if attempts >= max_attempts:
            break
        attempts += 1
        if card.authenticate(candidate, card.challenge()):
            return AuditResult(True, candidate, attempts, False)
    return AuditResult(False, None, attempts, attempts >= max_attempts)


def detect_clone(original: VirtualCard, clone: StaticClone) -> dict[str, object]:
    """Exercise defensive checks: identity collision, counter reuse, and live auth."""
    original_before = original.snapshot()
    challenge = original.challenge()
    original_ok = original.authenticate("lab-pass-07", challenge)
    clone_ok = clone.authenticate("lab-pass-07", challenge)
    original_after = original.snapshot()
    return {
        "uid_collision": original_before.uid == clone.snapshot.uid,
        "static_fingerprint_match": original_before.fingerprint == clone.snapshot.fingerprint,
        "counter_reuse": clone.snapshot.counter <= original_before.counter,
        "original_dynamic_auth": original_ok,
        "clone_dynamic_auth": clone_ok,
        "clone_rejected": not clone_ok,
        "counter_advanced_on_original": original_after.counter > original_before.counter,
    }


def run_demo() -> dict[str, object]:
    card = VirtualCard.create()
    weak_candidates = ["123456", "password", "lab-pass-07", "admin"]
    audit = bounded_password_audit(card, weak_candidates)
    clone = StaticClone(card.static_export())
    report = detect_clone(card, clone)
    return {
        "scope": "offline synthetic card only",
        "protocol": LAB_PROTOCOL,
        "audit": audit.__dict__,
        "clone_detection": report,
        "guardrails": {
            "hardware": False,
            "network": False,
            "real_card_protocol": False,
            "max_audit_attempts": MAX_AUDIT_ATTEMPTS,
            "secret_exported": False,
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="NNN offline virtual-card security lab")
    parser.add_argument("--demo", action="store_true", help="run the deterministic lab scenario")
    args = parser.parse_args()
    if not args.demo:
        parser.error("use --demo; this tool does not accept card readers, dumps, or network targets")
    print(json.dumps(run_demo(), indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
