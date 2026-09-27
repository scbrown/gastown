"""Offline controls for the CI partition gate; never starts Go or Docker."""

import json
import re
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import integration_partitions as partitions


class PartitionTests(unittest.TestCase):
    def setUp(self):
        self.full = {("cmd", name) for name in (
            "TestSchedulerDispatch", "TestScheduleBead_Closed", "TestNewlyAdded",
            "Example", "FuzzDecode", "BenchmarkEncode", "TestUnicode_é",
        )} | {("cmd/subpackage", "TestNewlyAdded")}

    def test_compiled_inventory_includes_examples_fuzz_and_package_identity(self):
        output = "\n".join(json.dumps(dict(Action="output", Package=pkg, Output=name + "\n"))
                           for pkg, name in self.full)
        output += '\n' + json.dumps(dict(Action="output", Package="cmd", Output="ok cmd 0.01s\n"))
        with patch.object(partitions.subprocess, "run", return_value=SimpleNamespace(
                returncode=0, stdout=output, stderr="")):
            self.assertEqual(partitions.inventory("."), self.full)

    def test_missing_overlap_extra_and_empty_are_refused(self):
        scheduler = {("cmd", "TestSchedulerDispatch")}
        rest = self.full - scheduler
        for a, b in ((set(), rest), (scheduler, set()),
                     (scheduler, rest - {("cmd", "TestNewlyAdded")}),
                     (scheduler, rest | scheduler),
                     (scheduler, rest | {("cmd", "TestInvented")})):
            with self.subTest(a=a, b=b), self.assertRaises(RuntimeError):
                partitions.verify(self.full, a, b)
        partitions.verify(self.full, scheduler, rest)

    def test_execution_reuses_verified_selectors_and_either_failure_stays_red(self):
        for returncodes in ((0, 0), (1, 0), (0, 1)):
            listed = []
            def inventory(pattern):
                listed.append(pattern)
                return {row for row in self.full if re.search(pattern, row[1])}
            with tempfile.TemporaryDirectory() as tmp, \
                    patch.object(partitions, "inventory", side_effect=inventory), \
                    patch.object(partitions.subprocess, "run", side_effect=[
                        SimpleNamespace(returncode=code) for code in returncodes]) as run:
                receipt = Path(tmp) / "proof.json"
                self.assertEqual(partitions.main(receipt), int(any(returncodes)))
                saved = json.loads(receipt.read_text())
                self.assertEqual(len(saved["full"]), len(self.full))
                self.assertEqual(run.call_count, 2)
                for call, pattern in zip(run.call_args_list, listed[1:]):
                    args = call.args[0]
                    self.assertEqual(args[args.index("-run") + 1], pattern)
                    self.assertIn("-timeout=15m", args)
                    self.assertIn("-tags=integration", args)
                    self.assertNotIn("-skip", args)

    def test_bad_inventory_prevents_any_test_invocation(self):
        with patch.object(partitions, "inventory", side_effect=[
                self.full, self.full, self.full]), \
                patch.object(partitions.subprocess, "run") as run:
            with self.assertRaises(RuntimeError): partitions.main()
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
