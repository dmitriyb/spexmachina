#!/usr/bin/env python3
"""Deterministic self-test of the spec-review scripts. No model, no network, a few seconds.

usage: selftest.py [--spec-dir DIR] [--spex BIN]

Runs against the repository's own spec and a scratch directory. Every assertion
names what it checks; the first failure stops the run with exit 1. Run it before
any review: a reviewer is the most expensive place to find a script bug.
"""
import json, os, re, shutil, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC, SPEX = "spec", "bin/spex"
PY = sys.executable

def sh(*args, expect=None):
    p = subprocess.run(list(args), capture_output=True, text=True)
    if expect is not None and p.returncode != expect:
        fail(f"{' '.join(str(a) for a in args)} exited {p.returncode}, expected {expect}\n{p.stdout[-800:]}\n{p.stderr[-800:]}")
    return p

def fail(msg):
    print("FAIL " + msg); sys.exit(1)

def ok(msg): print("ok   " + msg)

def scope(stage, *extra):
    return sh(PY, f"{HERE}/scope.py", "--spec-dir", SPEC, "--spex", SPEX, "--stage", stage, *extra, expect=0)

def read(p): return open(p, encoding="utf-8").read()

def transcript(path, events):
    """events: list of (kind, payload); kind in use-bash, use-read, result, usage."""
    with open(path, "w") as f:
        n = 0
        for kind, payload in events:
            n += 1
            if kind == "use-bash": blk = {"type": "tool_use", "id": f"t{n}", "name": "Bash", "input": {"command": payload}}
            elif kind == "use-read": blk = {"type": "tool_use", "id": f"t{n}", "name": "Read", "input": {"file_path": payload}}
            elif kind == "result": blk = {"type": "tool_result", "tool_use_id": f"t{n-1}", "content": payload}
            else: f.write(json.dumps({"message": {"usage": payload, "content": []}}) + "\n"); continue
            f.write(json.dumps({"message": {"content": [blk], "usage": {"input_tokens": 10, "cache_read_input_tokens": 100, "cache_creation_input_tokens": 5, "output_tokens": 3}}}) + "\n")

def main(argv):
    global SPEC, SPEX
    if "--spec-dir" in argv: SPEC = argv[argv.index("--spec-dir") + 1]
    if "--spex" in argv: SPEX = argv[argv.index("--spex") + 1]
    if not os.path.exists(SPEX): fail(f"binary {SPEX} missing; build it first")
    tmp = tempfile.mkdtemp(prefix="spec-review-selftest-")
    try:
        # 1. determinism: two stagings of the same diff are byte-identical in every derived file
        a, b = f"{tmp}/a", f"{tmp}/b"
        scope(a); scope(b)
        for f in ("SCOPE.tsv", "PAIRS.tsv", "CELLS.tsv", "READ-PLAN.tsv", "UNDECLARED.tsv"):
            if read(f"{a}/{f}") != read(f"{b}/{f}"): fail(f"{f} differs between two stagings of the same diff")
        slims = sorted(p for p in sh("find", a, "-name", "module.slim.json").stdout.split())
        if not slims: fail("no slim module files staged")
        for p in slims:
            if read(p) != read(p.replace(a, b, 1)): fail(f"{os.path.relpath(p, a)} differs between two stagings")
        ok(f"determinism: {len(slims)} slim files and 5 derived files identical across two stagings")
        # 2. read plan: every staged leaf and slim planned exactly once, rows under the limit unless a single file exceeds it
        cfg = json.load(open(f"{HERE}/review.json")); limit = cfg["read_call_bytes"]
        planned = [r.split("\t") for r in read(f"{a}/READ-PLAN.tsv").splitlines()[1:]]
        files = [f for r in planned for f in r[1].split(",")]
        if len(files) != len(set(files)): fail("a file appears twice in READ-PLAN.tsv")
        staged = {os.path.relpath(os.path.join(r_, f), a) for r_, _, fs in os.walk(a) for f in fs if f.endswith((".md", ".slim.json")) or f == "project.json"}
        if set(files) != staged: fail(f"read plan covers {len(files)} files, stage holds {len(staged)}: {sorted(staged ^ set(files))[:5]}")
        for r in planned:
            if int(r[2]) > limit and "," in r[1]: fail(f"plan row {r[0]} packs several files above the limit")
        ok(f"read plan: {len(planned)} rows cover all {len(staged)} staged files, none packed above {limit} bytes")
        # 3. expansion: --expand adds no seam rows, widens slim files, appends plan rows for new files only
        nodes = [l.split("\t") for l in read(f"{a}/SCOPE.tsv").splitlines()]
        frontier_mod = next((l[1] for l in nodes if l[0] == "frontier" and l[2] == "module"), None)
        seed = next((l[1] for l in nodes if l[0] == "frontier" and l[2] == "component" and l[5]), None)
        if not seed: fail("no frontier node to expand on")
        before_rows = len(read(f"{a}/SCOPE.tsv").splitlines()); before_plan = len(planned)
        req_before = {p: len(json.load(open(p)).get("requirements", [])) for p in slims}
        p = scope(a, "--expand", seed)
        if "0 seam leaves" not in p.stderr: fail("--expand derived seam rows: " + p.stderr.strip())
        after = read(f"{a}/SCOPE.tsv").splitlines()
        if len(after) <= before_rows: fail("expansion appended no SCOPE rows")
        if any(l.startswith("seam\t") for l in after[before_rows:]): fail("expansion appended seam rows")
        for pth, n in req_before.items():
            if os.path.exists(pth) and len(json.load(open(pth)).get("requirements", [])) < n: fail(f"expansion narrowed {os.path.relpath(pth, a)}")
        planned2 = [r.split("\t") for r in read(f"{a}/READ-PLAN.tsv").splitlines()[1:]]
        files2 = [f for r in planned2 for f in r[1].split(",")]
        if len(files2) != len(set(files2)): fail("expansion re-planned an already planned file")
        if len(planned2) <= before_plan: fail("expansion on a component with a leaf appended no plan rows")
        if int(planned2[-1][0]) <= before_plan: fail("expansion plan rows do not continue the numbering")
        ok(f"expansion on {seed}: {len(after) - before_rows} rows, {len(planned2) - before_plan} plan rows, no seams, slim files never narrowed")
        # 4. findings, cells, verdicts and the audit, on synthetic transcripts
        leaf = next(f for f in files if f.endswith(".md") and os.path.basename(f).startswith("arch_"))
        leaf_text = read(f"{a}/{leaf}")
        sentence = next(s for s in re.split(r"(?<=[.!?])\s+", leaf_text.replace("\n", " ")) if 60 < len(s) < 200 and "[[" not in s)
        slim = os.path.relpath(slims[0], a); slim_doc = json.load(open(slims[0]))
        desc = next(r["description"] for r in slim_doc.get("requirements", []) if len(r.get("description", "")) > 80)
        jquote = desc[:70]
        for line in read(f"{a}/PAIRS.tsv").splitlines()[1:]: pass
        pairs = read(f"{a}/PAIRS.tsv").splitlines()
        with open(f"{a}/PAIRS.tsv", "w") as f: f.write(pairs[0] + "\n" + "\n".join(l + "clean" for l in pairs[1:]) + "\n")
        cells = read(f"{a}/CELLS.tsv").splitlines()
        with open(f"{a}/CELLS.tsv", "w") as f: f.write(cells[0] + "\n" + "\n".join(l.rstrip("\t") + "\tINDEPENDENT\t" for l in cells[1:]) + "\n")
        with open(f"{a}/FINDINGS.tsv", "a") as f:
            f.write("\t".join(["F1", "x", leaf, sentence, "y", slim, jquote, "leaf against requirement", "fix"]) + "\n")
            f.write("\t".join(["F2", "x", leaf, '"' + sentence.replace('"', '""') + '"', "y", slim, '"' + jquote.replace('"', '""') + '"', "csv-quoted copy", "fix"]) + "\n")
        t_clean = f"{tmp}/clean.jsonl"
        transcript(t_clean, [("use-bash", f'cd "{a}" && cat {leaf}'), ("result", leaf_text), ("usage", {"input_tokens": 1, "cache_read_input_tokens": 2, "cache_creation_input_tokens": 3, "output_tokens": 4})])
        p = sh(PY, f"{HERE}/audit-reads.py", t_clean, a, "--spec-dir", SPEC)
        if p.returncode != 0: fail("audit rejected a clean run:\n" + p.stdout)
        ok("audit: clean run accepted; plain, CSV-quoted and JSON quotes all verified verbatim")
        staged_now = {os.path.relpath(os.path.join(r_, f), a) for r_, _, fs in os.walk(a) for f in fs}  # after the expansion
        outside_leaf = sorted(set(os.path.relpath(os.path.join(r_, f), SPEC) for r_, _, fs in os.walk(SPEC) for f in fs if f.startswith("arch_")) - staged_now)
        if not outside_leaf: fail("no unstaged leaf available for the outside-read case")
        cases = {
            "OUTSIDE": [("use-bash", "cat something"), ("result", read(f"{SPEC}/{outside_leaf[0]}"))],
            "DUPLICATE": [("use-bash", "cat x"), ("result", leaf_text), ("use-bash", "cat x"), ("result", leaf_text)],
            "OVERFLOW": [("use-bash", "cat many"), ("result", "<persisted-output>\nOutput too large (99KB). Full output saved to: /x/y.txt")],
            "PERSISTED": [("use-read", "/home/u/.claude/projects/p/tool-results/abc.txt"), ("result", "x")],
            "SUSPECT": [("use-bash", f"for m in {SPEC}/*/module.json; do cat $m; done"), ("result", "x")],
        }
        for tag, ev in cases.items():
            t = f"{tmp}/{tag}.jsonl"; transcript(t, ev)
            p = sh(PY, f"{HERE}/audit-reads.py", t, a, "--spec-dir", SPEC)
            if p.returncode == 0 or tag not in p.stdout: fail(f"audit missed the {tag} case:\n{p.stdout}")
        ok("audit: OUTSIDE (by content), DUPLICATE, OVERFLOW, PERSISTED and SUSPECT each rejected")
        with open(f"{a}/FINDINGS.tsv", "a") as f: f.write("\t".join(["F3", "x", leaf, "this sentence is not in the leaf at all", "", "", "", "fake", "fix"]) + "\n")
        p = sh(PY, f"{HERE}/audit-reads.py", t_clean, a, "--spec-dir", SPEC)
        if p.returncode == 0 or "QUOTE" not in p.stdout: fail("audit accepted a finding whose quote is not in the corpus")
        ok("audit: a finding with a fabricated quote rejected")
        # 5. passages and merge
        p = sh(PY, f"{HERE}/passages.py", a, expect=0)
        pas = read(f"{a}/PASSAGES.tsv")
        if pas.count("NOT FOUND") != 1: fail(f"passages: expected exactly the fabricated quote NOT FOUND, got {pas.count('NOT FOUND')}")
        if max(len(l) for l in pas.splitlines()) > 3000: fail("passages: a passage exceeds the bound")
        ok("passages: leaf and JSON quotes located and bounded; the fabricated one reported NOT FOUND")
        shutil.copytree(a, f"{tmp}/c", ignore=shutil.ignore_patterns("FINDINGS.tsv", "PASSAGES.tsv"))
        with open(f"{tmp}/c/FINDINGS.tsv", "w") as f:
            f.write(read(f"{a}/FINDINGS.tsv").splitlines()[0] + "\n")
            f.write("\t".join(["G1", "x", leaf, sentence[10:-5], "y", slim, jquote[:50], "leaf against requirement, other words", "fix"]) + "\n")
        p = sh(PY, f"{HERE}/merge-findings.py", f"{tmp}/m", a, f"{tmp}/c", expect=0)
        merged = read(f"{tmp}/m/FINDINGS.tsv").splitlines()[1:]
        both = [l for l in merged if "," in l.split("\t")[9]]
        if len(both) < 1: fail("merge did not unify the same finding quoted with different spans:\n" + p.stdout)
        ok(f"merge: {len(merged)} rows, {len(both)} recognised as shared across the two reviews")
        # 6. cost tool runs
        p = sh(PY, f"{HERE}/cost.py", t_clean, expect=0)
        if "turns x context" not in p.stdout: fail("cost.py printed no figures")
        ok("cost: figures printed")
        print("selftest: all checks passed")
        return 0
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
