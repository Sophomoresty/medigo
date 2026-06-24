#!/usr/bin/env python3
"""Dispatch 18 sites across 6 codex workers for medigo-extractors-v2 batch 1.

Layout: each worker takes 1 csslcloud site + 2 single-API sites.
"""
import os
import json

ASSIGNMENTS = [
    {
        "worker": 1,
        "branch": "work/v2-batch1-w1",
        "worktree": os.path.expanduser("~/code/medigo-w1"),
        "sites": [
            {"name": "jianshe99", "tier": "csslcloud", "source_dir": "Jianshe99"},
            {"name": "ahu", "tier": "single-api", "source_dir": "Ahu"},
            {"name": "cctalk", "tier": "single-api", "source_dir": "cctalk"},
        ],
    },
    {
        "worker": 2,
        "branch": "work/v2-batch1-w2",
        "worktree": os.path.expanduser("~/code/medigo-w2"),
        "sites": [
            {"name": "med66", "tier": "csslcloud", "source_dir": "Med66"},
            {"name": "smartedu", "tier": "single-api", "source_dir": "Smartedu"},
            {"name": "htknow", "tier": "single-api", "source_dir": "Htknow"},
        ],
    },
    {
        "worker": 3,
        "branch": "work/v2-batch1-w3",
        "worktree": os.path.expanduser("~/code/medigo-w3"),
        "sites": [
            {"name": "houda", "tier": "csslcloud", "source_dir": "Houda"},
            {"name": "open163", "tier": "single-api", "source_dir": "Open163"},
            {"name": "koolearn", "tier": "single-api", "source_dir": "Koolearn"},
        ],
    },
    {
        "worker": 4,
        "branch": "work/v2-batch1-w4",
        "worktree": os.path.expanduser("~/code/medigo-w4"),
        "sites": [
            {"name": "qihang", "tier": "csslcloud+bokecc", "source_dir": "Qihang"},
            {"name": "nmkjxy", "tier": "single-api", "source_dir": "Nmkjxy"},
            {"name": "cnmooc", "tier": "single-api", "source_dir": "Cnmooc"},
        ],
    },
    {
        "worker": 5,
        "branch": "work/v2-batch1-w5",
        "worktree": os.path.expanduser("~/code/medigo-w5"),
        "sites": [
            {"name": "shanxiang", "tier": "csslcloud", "source_dir": "Shanxiang"},
            {"name": "sanjieke", "tier": "single-api", "source_dir": "Sanjieke"},
            {"name": "lexueyun", "tier": "single-api", "source_dir": "Lexueyun"},
        ],
    },
    {
        "worker": 6,
        "branch": "work/v2-batch1-w6",
        "worktree": os.path.expanduser("~/code/medigo-w6"),
        "sites": [
            {"name": "aishangke", "tier": "csslcloud", "source_dir": "Aishangke"},
            {"name": "chaoge", "tier": "csslcloud", "source_dir": "Chaoge"},
            {"name": "caixuetang", "tier": "single-api", "source_dir": "Caixuetang"},
        ],
    },
]


def main():
    out = os.path.expanduser("~/code/medigo/.codex/tasks/medigo-extractors-v2/dispatch.json")
    with open(out, "w") as f:
        json.dump(ASSIGNMENTS, f, indent=2, ensure_ascii=False)
    print(f"Wrote {out}")
    print(f"Total: {len(ASSIGNMENTS)} workers, {sum(len(w['sites']) for w in ASSIGNMENTS)} sites")
    for w in ASSIGNMENTS:
        sites = ", ".join(f"{s['name']} ({s['tier']})" for s in w["sites"])
        print(f"  worker-{w['worker']}: {sites}")


if __name__ == "__main__":
    main()
