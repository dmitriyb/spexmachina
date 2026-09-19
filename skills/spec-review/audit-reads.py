#!/usr/bin/env python3
"""Audit a review subagent's transcript against its staging directory.

usage: audit-reads.py <transcript.jsonl> <stage-dir> [--spec-dir DIR]

Two detections, because a reviewer with a shell can reach a file many ways:

1. every spec path a Read call or a shell command names literally, and every
   glob or loop over the spec directory in a shell command;
2. every spec file NOT in the stage whose content shows up in a tool result —
   a fingerprint of the file (its first lines, whitespace-normalised) searched
   through everything the tools returned. This catches `cat "$DIR/x.md"`, loops
   and dumps that name no literal path.

Three more checks, because waste is a failure too: a staged leaf whose content
appears in more than one tool result was read twice; a tool result the harness
persisted because it was too large is an overflow — the reviewer batched past
the limit and will read the file back; a Read of a persisted tool-result file is
that re-read. Batching several files into one call is fine below the limit.

And the reviewer's own data must be honest: every row of `PAIRS.tsv` carries a
verdict; every quote in `FINDINGS.tsv` and `CELLS.tsv` occurs verbatim in the
staged file it names; every finding number a pair or cell cites exists in
`FINDINGS.tsv`; every `UNDECIDED` cell cites a finding.

Prints the reads in order and every failure. Exit 0 when every read was staged
exactly once, nothing was batched or re-read, and every pair has a verdict;
1 otherwise. This is the acceptance test of a review: a reviewer that read
outside the stage did not walk, whatever its report says, and a reviewer that
left a pair unjudged did not review it.
"""
import json, os, re, sys

def norm(t): return re.sub(r"\s+", " ", re.sub(r"(?m)^\s*\d+\t", "", t)).strip()  # Read output carries line-number prefixes
def unq(f):
    """A field a CSV writer wrapped in double quotes, inner quotes doubled: undo it."""
    f = f.strip("\r")
    return f[1:-1].replace('""', '"') if len(f) >= 2 and f[0] == '"' and f[-1] == '"' else f


def main(argv):
    if len(argv) < 2: print(__doc__); return 2
    transcript, stage = argv[0], argv[1].rstrip("/")
    spec_dir = argv[argv.index("--spec-dir") + 1].rstrip("/") if "--spec-dir" in argv else "spec"
    staged = set()
    for root, _, files in os.walk(stage):
        for f in files:
            rel = os.path.relpath(os.path.join(root, f), stage)
            if rel != "SCOPE.tsv": staged.add(rel)
    # every spec file, with a fingerprint for the unstaged ones
    fingerprints, staged_fp = {}, {}
    for root, _, files in os.walk(spec_dir):
        if "/proposals" in root or root.endswith("/proposals"): continue
        for f in files:
            if not f.endswith((".md", ".json")): continue
            rel = os.path.relpath(os.path.join(root, f), spec_dir)
            text = norm(open(os.path.join(root, f), encoding="utf-8", errors="replace").read())
            # a slim extract shares its head with the full module.json it stands for, so
            # the full file is fingerprinted by its tail, which the extract does not carry
            slim_staged = rel.endswith("module.json") and rel[:-len("module.json")] + "module.slim.json" in staged
            fp = text[-160:] if slim_staged else text[:160]
            if len(fp) < 60: continue
            if rel in staged: staged_fp[rel] = fp
            else: fingerprints[rel] = fp
    for root, _, files in os.walk(stage):  # staged files with no spec counterpart: slim extracts
        for f in files:
            rel = os.path.relpath(os.path.join(root, f), stage)
            if rel.endswith(".slim.json") and rel not in staged_fp:
                fp = norm(open(os.path.join(root, f), encoding="utf-8", errors="replace").read())[:160]
                if len(fp) >= 60: staged_fp[rel] = fp
    lit = re.compile(re.escape(spec_dir) + r"/[A-Za-z0-9_./-]+\.(?:md|json)")
    # globs and loops count only when they reach the spec directory itself; a loop over the stage is fine
    glob = re.compile(re.escape(spec_dir) + r"/[^ \"'`;|)]*[*?][^ \"'`;|)]*")
    loop = re.compile(r"\bfor\s+\w+\s+in\b[^;]*" + re.escape(spec_dir) + r"/")
    order, outside, suspicious = [], [], []
    results, persisted = [], []
    stage_reads = {}  # staged path -> number of tool calls naming it (catches partial reads such as jq)
    stage_pat = re.compile(r"(?:" + re.escape(stage) + r"|\$\{?[A-Za-z_]+\}?)/([A-Za-z0-9_./-]+\.(?:md|json))")  # literal stage path or a shell variable standing for it
    for line in open(transcript, errors="replace"):
        try: ev = json.loads(line)
        except Exception: continue
        if not isinstance(ev, dict): continue
        msg = ev.get("message") or ev
        content = msg.get("content") if isinstance(msg, dict) else None
        if not isinstance(content, list): continue
        for blk in content:
            if not isinstance(blk, dict): continue
            if blk.get("type") == "tool_use":
                inp, name = blk.get("input", {}), blk.get("name")
                t = inp.get("file_path", "") if name == "Read" else (inp.get("command", "") or "") if name == "Bash" else ""
                if name == "Read" and "/tool-results/" in t: persisted.append(t.rsplit("/", 1)[-1])
                for m in lit.findall(t):
                    rel = m[len(spec_dir) + 1:]
                    if rel.startswith("proposals/") or not os.path.exists(os.path.join(spec_dir, rel)): continue  # proposals are not leaves; a named path that does not exist was not read
                    if rel not in order: order.append(rel)
                    if rel not in staged and rel not in outside: outside.append(rel)
                for g in glob.findall(t): suspicious.append("glob: " + g)
                for rel in set(stage_pat.findall(t)):
                    if rel in staged and rel not in ("SCOPE.tsv", "PAIRS.tsv", "FINDINGS.tsv", "CELLS.tsv"): stage_reads[rel] = stage_reads.get(rel, 0) + 1
                if loop.search(t): suspicious.append("loop: " + t.replace("\n", " ")[:120])
            elif blk.get("type") == "tool_result":
                c = blk.get("content")
                if isinstance(c, list): c = " ".join(x.get("text", "") for x in c if isinstance(x, dict))
                if isinstance(c, str): results.append(norm(c))
    blob = " ".join(results)
    by_content = [rel for rel, fp in fingerprints.items() if fp in blob]
    for rel in by_content:
        if rel not in outside: outside.append(rel)
    dup = {rel: blob.count(fp) for rel, fp in staged_fp.items() if blob.count(fp) > 1}
    for rel, c in stage_reads.items():
        if c > 1 and rel not in dup: dup[rel] = c
    batches = [i for i, r in enumerate(results) if "output too large" in r or "<persisted-output>" in r]
    # quotes in FINDINGS.tsv and CELLS.tsv must be verbatim in the staged file they name
    def json_strings(obj, out):
        if isinstance(obj, dict):
            for v in obj.values(): json_strings(v, out)
        elif isinstance(obj, list):
            for v in obj: json_strings(v, out)
        elif isinstance(obj, str): out.append(obj)
        return out
    def staged_text(rel):
        p = os.path.join(stage, rel)
        if not os.path.exists(p): return None
        raw = open(p, encoding="utf-8", errors="replace").read()
        if rel.endswith(".json"):  # a quote from a JSON field is matched against the decoded strings, not the escaped bytes
            try: return norm(raw + " " + " ".join(json_strings(json.loads(raw), [])))
            except Exception: pass
        return norm(raw)
    bad_quotes, finding_ids, cited = [], set(), []
    fpath = os.path.join(stage, "FINDINGS.tsv")
    if os.path.exists(fpath):
        for l in open(fpath):
            if l.startswith("id\t"): continue
            c = [unq(x) for x in l.rstrip("\n").split("\t")]
            if len(c) < 9: bad_quotes.append(f"finding row malformed: {l[:60]}"); continue
            finding_ids.add(c[0])
            for fid, rel, q in ((c[0], c[2], c[3]), (c[0], c[5], c[6])):
                if not q.strip(): continue
                t = staged_text(rel)
                if t is None or norm(q) not in t: bad_quotes.append(f"finding {fid}: quote not verbatim in {rel}: {q[:60]}")
    cpath = os.path.join(stage, "CELLS.tsv")
    if os.path.exists(cpath):
        for l in open(cpath):
            if l.startswith("leaf\t"): continue
            c = [unq(x) for x in l.rstrip("\n").split("\t")]
            if len(c) < 5: bad_quotes.append(f"cell row malformed: {l[:60]}"); continue
            if c[3].strip().upper() == "INDEPENDENT": continue  # the two flags never interact; no sentence is owed
            if c[3].strip().upper() == "UNDECIDED":
                if not c[4].strip(): bad_quotes.append(f"cell {c[0]} {c[1]} x {c[2]}: undecided without a finding")
                else: cited.append(c[4].strip())
            else:
                t = staged_text(c[0])
                if t is None or norm(c[3]) not in t: bad_quotes.append(f"cell {c[0]} {c[1]} x {c[2]}: deciding quote not verbatim")
    unjudged = []
    ppath = os.path.join(stage, "PAIRS.tsv")
    if os.path.exists(ppath):
        for l in open(ppath):
            if l.startswith("kind\t"): continue
            cols = [unq(x) for x in l.rstrip("\n").split("\t")]
            if len(cols) < 8 or not cols[7].strip(): unjudged.append(cols[0] + " " + cols[2] + " x " + cols[5])
            elif cols[7].strip().lower() != "clean": cited.append(cols[7].strip())
    for x in cited:
        for fid in re.split(r"[,\s]+", x):
            if fid and fid not in finding_ids: bad_quotes.append(f"verdict cites finding {fid} that FINDINGS.tsv does not carry")
    print(f"staged: {len(staged)} files; named reads: {len(order)}; outside the stage: {len(outside)} (by path {sum(1 for r in outside if r in order)}, by content {len(by_content)}); globs/loops: {len(suspicious)}; leaves read more than once: {len(dup)}; overflowed reads: {len(batches)}; persisted re-reads: {len(persisted)}; pairs without verdict: {len(unjudged)}; findings: {len(finding_ids)}; quote/citation failures: {len(bad_quotes)}")
    for r in order: print(("OUTSIDE " if r in outside else "staged  ") + r)
    for r in by_content:
        if r not in order: print("OUTSIDE " + r + "  (content seen in a tool result)")
    for s_ in suspicious: print("SUSPECT " + s_)
    for rel, c in sorted(dup.items()): print(f"DUPLICATE {rel} read {c} times")
    for i in batches: print(f"OVERFLOW tool result #{i} was persisted by the harness")
    for b in bad_quotes: print("QUOTE    " + b)
    for p in persisted: print("PERSISTED re-read of " + p)
    for u in unjudged[:20]: print("UNJUDGED " + u)
    if len(unjudged) > 20: print(f"UNJUDGED ... {len(unjudged) - 20} more")
    return 1 if outside or suspicious or dup or batches or persisted or unjudged or bad_quotes else 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
