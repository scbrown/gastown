#!/usr/bin/env python3
"""Prove exhaustive Go inventories before running two integration invocations.

Each invocation has its own isolated test database and the existing 15m timeout.
The combined suite can take longer than the former single 15m invocation.
"""

import json
import re
import subprocess
from pathlib import Path

PACKAGES = ["./internal/cmd/..."]
GO_FLAGS = ["-tags=integration", "-timeout=15m"]
LISTED_NAME = re.compile(r"(?:Test|Benchmark|Fuzz|Example)\w*\Z")


def inventory(pattern):
    result = subprocess.run(
        ["go", "test", "-json", *GO_FLAGS, "-list", pattern, *PACKAGES],
        capture_output=True, text=True, timeout=300, check=False,
    )
    if result.returncode:
        raise RuntimeError("Go test inventory failed:\n" + result.stdout + result.stderr)
    names = set()
    for line in result.stdout.splitlines():
        event = json.loads(line)
        name = event.get("Output", "").strip()
        if event.get("Action") == "output" and LISTED_NAME.fullmatch(name):
            names.add((event["Package"], name))
    return names


def selectors(full):
    scheduler = {name for _, name in full
                 if name.startswith(("TestScheduler", "TestScheduleBead_"))}
    rest = {name for _, name in full} - scheduler
    if not scheduler or not rest:
        raise RuntimeError("Both integration partitions must be nonempty")
    return {key: "^(?:" + "|".join(re.escape(n) for n in sorted(names)) + ")$"
            for key, names in (("scheduler", scheduler), ("rest", rest))}


def verify(full, scheduler, rest):
    if not full or not scheduler or not rest:
        raise RuntimeError("Empty Go test inventory or partition")
    overlap = scheduler & rest
    missing = full - (scheduler | rest)
    extra = (scheduler | rest) - full
    if overlap or missing or extra:
        raise RuntimeError(f"Partition mismatch: overlap={sorted(overlap)}, "
                           f"missing={sorted(missing)}, extra={sorted(extra)}")


def main(receipt=Path("integration-partitions.json")):
    # Include all names Go lists, including examples/fuzz seeds. Benchmarks stay
    # listed but are not enabled: neither the original nor new command uses -bench.
    full = inventory(".")
    patterns = selectors(full)
    groups = {key: inventory(pattern) for key, pattern in patterns.items()}
    verify(full, groups["scheduler"], groups["rest"])
    receipt.write_text(json.dumps({
        "full": sorted(full), "partitions": {k: sorted(v) for k, v in groups.items()},
        "selectors": patterns,
    }, indent=2) + "\n")
    print(f"Partition proof: full={len(full)}, scheduler={len(groups['scheduler'])}, "
          f"rest={len(groups['rest'])}, overlap=0, missing=0", flush=True)

    failed = False
    # Execute the identical selectors just proved against Go's compiled list.
    # Run both even if one fails; either failure keeps Integration Tests red.
    for key, pattern in patterns.items():
        print(f"Running {key} integration partition (timeout=15m)", flush=True)
        result = subprocess.run([
            "gotestsum", "--format", "testname", "--junitfile", f"junit-integration-{key}.xml",
            "--", *GO_FLAGS, "-run", pattern, "-v", *PACKAGES,
        ], check=False)
        failed |= result.returncode != 0
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(main())
