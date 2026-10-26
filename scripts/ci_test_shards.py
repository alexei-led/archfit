#!/usr/bin/env python3
"""Split the package list from `go list ./...` into balanced CI test shards.

Round-robin is not enough for this suite: three packages are most of its measured
runtime, so a shard that happens to catch two of them is slower than the single
job it replaced. Costs are warm `go test -race` seconds measured on a GitHub
ubuntu-24.04 runner; an unlisted package costs DEFAULT and is still scheduled, so
a new package can never be dropped from CI. Longest-processing-time greedy keeps
the shards within about 10% of each other.

usage: go list ./... | ci_test_shards.py --shard 2 --total 3
"""

import argparse
import sys

DEFAULT_COST = 1.2

# cmd/archfit is 56s of the suite on its own, cannot be sharded by package, and is
# split by test name across its own runners instead (see the CI job).
SPLIT = ("github.com/alexei-led/archfit/v3/cmd/archfit",)

# Measured warm per-package seconds, slowest first. Refresh with the "Run tests"
# durations in a CI log; only the relative order matters for the split.
COSTS = {
    "github.com/alexei-led/archfit/v3/internal/toolrun": 26.0,
    "github.com/alexei-led/archfit/v3/internal/extract/ts": 25.0,
    "github.com/alexei-led/archfit/v3/internal/extract/scip": 21.0,
    "github.com/alexei-led/archfit/v3/internal/extract/py": 13.0,
    "github.com/alexei-led/archfit/v3/internal": 11.0,
    "github.com/alexei-led/archfit/v3/internal/extract/golang": 4.5,
}


def cost(package):
    return COSTS.get(package, DEFAULT_COST)


def shard_of(packages, total, shard):
    """Return the packages assigned to a one-based shard number."""
    packages = [name for name in packages if name not in SPLIT]
    loads = [0.0] * total
    members = [[] for _ in range(total)]
    for package in sorted(packages, key=lambda name: (-cost(name), name)):
        target = min(range(total), key=lambda index: (loads[index], index))
        members[target].append(package)
        loads[target] += cost(package)
    return members[shard - 1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--shard", type=int, required=True)
    parser.add_argument("--total", type=int, required=True)
    args = parser.parse_args()
    if args.total < 1 or not 1 <= args.shard <= args.total:
        parser.error("--shard must be within 1..--total")
    packages = [line.strip() for line in sys.stdin if line.strip()]
    for package in shard_of(packages, args.total, args.shard):
        print(package)


if __name__ == "__main__":
    main()

