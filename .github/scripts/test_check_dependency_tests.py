import importlib.util
from pathlib import Path
import re
import unittest

spec = importlib.util.spec_from_file_location(
    "check", Path(__file__).with_name("check-dependency-tests.py")
)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


class DependencyChecks(unittest.TestCase):
    def test_required_names_exist_in_current_source(self):
        root = Path(__file__).resolve().parents[2] / "backend" / "internal"
        for package, name in check.REQUIRED:
            source = "\n".join(path.read_text() for path in (root / package).glob("*_test.go"))
            self.assertRegex(source, r"(?m)^func " + re.escape(name) + r"\(t \*testing.T\)", name)

    def test_requires_run_and_pass_for_every_exact_name(self):
        events = [
            {"Package": "example/internal/" + package, "Test": name, "Action": action}
            for package, name in check.REQUIRED
            for action in ("run", "pass")
        ]
        self.assertFalse(check.missing_tests(events))
        self.assertTrue(check.missing_tests([]))
        self.assertTrue(check.missing_tests(events[1:]))
        self.assertTrue(check.missing_tests(events[:-1]))
        for action in ("skip", "fail"):
            self.assertTrue(check.missing_tests(events + [dict(events[-1], Action=action)]))
        self.assertTrue(check.missing_tests([dict(e, Test=e["Test"] + "Renamed") for e in events]))


if __name__ == "__main__":
    unittest.main()
