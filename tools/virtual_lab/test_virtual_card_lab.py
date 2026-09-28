import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from virtual_card_lab import MAX_AUDIT_ATTEMPTS, StaticClone, VirtualCard, bounded_password_audit, detect_clone


class VirtualCardLabTests(unittest.TestCase):
    def test_bounded_audit_finds_only_synthetic_lab_secret(self):
        card = VirtualCard.create()
        result = bounded_password_audit(card, ["wrong-1", "wrong-2", "lab-pass-07"])
        self.assertTrue(result.found)
        self.assertEqual(result.candidate, "lab-pass-07")
        self.assertEqual(result.attempts, 3)

    def test_audit_has_hard_attempt_cap(self):
        card = VirtualCard.create()
        result = bounded_password_audit(card, (f"candidate-{i}" for i in range(1000)))
        self.assertFalse(result.found)
        self.assertEqual(result.attempts, MAX_AUDIT_ATTEMPTS)
        self.assertTrue(result.exhausted)

    def test_static_clone_cannot_pass_dynamic_authentication(self):
        card = VirtualCard.create()
        clone = StaticClone(card.static_export())
        report = detect_clone(card, clone)
        self.assertTrue(report["uid_collision"])
        self.assertTrue(report["static_fingerprint_match"])
        self.assertFalse(report["clone_dynamic_auth"])
        self.assertTrue(report["clone_rejected"])
        self.assertTrue(report["counter_advanced_on_original"])

    def test_unseen_challenge_is_rejected(self):
        card = VirtualCard.create()
        self.assertFalse(card.authenticate("lab-pass-07", b"not-issued-by-card"))

    def test_invalid_budget_is_rejected(self):
        with self.assertRaises(ValueError):
            bounded_password_audit(VirtualCard.create(), [], max_attempts=MAX_AUDIT_ATTEMPTS + 1)


if __name__ == "__main__":
    unittest.main()
