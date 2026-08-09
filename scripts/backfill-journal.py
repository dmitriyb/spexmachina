#!/usr/bin/env python3
"""backfill-journal.py — one-shot seed of spec/.history.jsonl from .bead-map.json history.

The task-journal migration (spec/proposals/2026-08-01-task-journal.md) replaced the
node-keyed .bead-map.json with an append-only event journal, and specified a one-shot
backfill: replay the committed versions of the map plus the tracker's closure state
into an initial journal, then delete the map in the same change. This is that script.
It runs once; its output is reviewed and committed like any migration.

Scope decisions, recorded here because the output is the migration:

- Replay starts at the identity-hash migration of the map (9d3c8838, 2026-04-11):
  earlier versions key records by pre-hash node ids the journal-line schema cannot
  express (node must be 12-hex). Every pairing still alive at that boundary appears
  in that version, so nothing live is lost; pairings retired before it were already
  dark under the current tooling.
- Event ids are "<map-version-commit>:bf-<record-id>" — unique, stable, and shaped
  like the reconciler's "<git_head>:<op_id>" derivation. git_head on each event is
  the map-version commit: the journal stores pointers, and that commit is the
  closest recorded head for the change the record row witnessed.
- proposal on backfilled change events is the literal string "backfill": the map
  never recorded which proposal drove a change, and inventing one would be false.
- A record whose bead_id changes seeds a new change event + task_created (last-wins
  fold semantics make the latest pairing authoritative). A record whose content
  hash or metadata changes seeds a modified event with before/after hashes and no
  receipt. A retired record whose node exists neither later in the map nor in the
  current spec seeds a removed event (name recovery for the removal sweep).
- task_closed receipts come from .beads/issues.jsonl status at run time.
- PREMAP_PAIRINGS seeds nodes implemented before the map existed and therefore
  absent from every map version. Each entry is evidence-backed: the cited commit
  created the component's source file and names the bead in its message. Without
  these, emit's resolver hard-errors on any dep reaching such a node.
- Epic pairings (task_created keyed by proposal slug) are seeded only where an
  epic-type bead is confidently attributable to a proposal stem; each such
  inference is printed for review.

Usage: scripts/backfill-journal.py   (from the repository root; writes spec/.history.jsonl)
"""

import json
import subprocess
import sys
from pathlib import Path

MAP_FILE = ".bead-map.json"
JOURNAL = Path("spec/.history.jsonl")
HASH_ERA_START = "9d3c8838e8747c41f0a305f7df90b0582dd4dc5d"

# node hash -> (name, node_type, module, bead id, evidence commit)
PREMAP_PAIRINGS = {
    "24180f55c0b4": ("Registrar", "component", "proposal", "spexmachina-das",
                     "0efb835"),  # created proposal/register.go, message names the bead
    "cc8adc823719": ("TemplateProvider", "component", "proposal", "spexmachina-das",
                     "0efb835"),  # created proposal/template.go, same bead
}


def run(*args):
    return subprocess.run(args, capture_output=True, text=True, check=True).stdout


def map_versions():
    """Chronological (sha) list of map versions from the hash era onward."""
    shas = run("git", "log", "--format=%H", "--reverse", "--", MAP_FILE).split()
    start = shas.index(HASH_ERA_START)
    return shas[start:]


def records_at(sha):
    raw = run("git", "show", f"{sha}:{MAP_FILE}")
    data = json.loads(raw)
    return {r["id"]: r for r in data["records"]}


def infer_node_type(rec):
    if rec.get("node_type"):
        return rec["node_type"]
    leaf = Path(rec.get("content_file", "")).name
    for prefix, t in (("arch_", "component"), ("flow_", "data_flow"), ("test_", "test_section")):
        if leaf.startswith(prefix):
            return t
    sys.exit(f"backfill: record {rec['id']} has no node_type and an uninferable leaf {leaf!r}")


def current_spec_nodes():
    raw = run("bin/spex", "render", "--format", "json", "--slim")
    return {n["id"] for n in json.loads(raw)["nodes"]}


def tracker_state():
    """bead id -> (status, issue_type, title) from the tracker's jsonl."""
    state = {}
    for line in Path(".beads/issues.jsonl").read_text().splitlines():
        b = json.loads(line)
        state[b["id"]] = (b.get("status"), b.get("issue_type"), b.get("title", ""))
    return state


def material_change(a, b):
    fields = ("spec_hash", "component", "module", "content_file")
    return any(a.get(f) != b.get(f) for f in fields)


def change_event(sha, rec, kind, before, after):
    return {
        "event": kind,
        "eid": f"{sha}:bf-{rec['id']}" + ("-rm" if kind == "removed" else ""),
        "node": rec["spec_node_id"],
        "name": rec.get("component", ""),
        "node_type": infer_node_type(rec),
        "module": rec.get("module", ""),
        "before": before,
        "after": after,
        "git_head": sha,
        "proposal": "backfill",
    }


def main():
    if JOURNAL.exists():
        sys.exit(f"backfill: {JOURNAL} already exists; this script runs once")

    versions = map_versions()
    spec_nodes = current_spec_nodes()
    tracker = tracker_state()

    lines = []            # journal lines, in order
    seen_nodes = set()    # node hashes that already have a change event
    created = []          # (eid, task_id) for closure pass
    epic_pairs = {}       # proposal slug -> latest bead id (from map epic rows)

    def is_node_hash(s):
        return len(s) == 12 and all(c in "0123456789abcdef" for c in s)

    for node, (name, node_type, module, bead, evidence) in sorted(PREMAP_PAIRINGS.items()):
        head = run("git", "rev-parse", evidence).strip()
        ev = {
            "event": "added", "eid": f"{head}:bf-premap-{node}", "node": node,
            "name": name, "node_type": node_type, "module": module,
            "before": None, "after": None, "git_head": head, "proposal": "backfill",
        }
        lines.append(ev)
        lines.append({"event": "task_created", "for": ev["eid"], "task_id": bead})
        created.append((ev["eid"], bead))
        seen_nodes.add(node)
        print(f"pre-map pairing: {node} ({module}/{name}) -> {bead} (evidence {evidence})")

    prev = {}
    for sha in versions:
        cur = records_at(sha)
        for rid in sorted(cur):
            rec, old = cur[rid], prev.get(rid)
            if not is_node_hash(rec["spec_node_id"]):
                # Epic row: spec_node_id is a proposal slug. Pair by slug, last wins.
                epic_pairs[rec["spec_node_id"]] = rec["bead_id"]
                continue
            if old is None or old["bead_id"] != rec["bead_id"]:
                kind = "modified" if rec["spec_node_id"] in seen_nodes else "added"
                before = old.get("spec_hash") if old else None
                ev = change_event(sha, rec, kind, before, rec.get("spec_hash"))
                lines.append(ev)
                lines.append({"event": "task_created", "for": ev["eid"], "task_id": rec["bead_id"]})
                created.append((ev["eid"], rec["bead_id"]))
                seen_nodes.add(rec["spec_node_id"])
            elif material_change(old, rec):
                lines.append(change_event(sha, rec, "modified", old.get("spec_hash"), rec.get("spec_hash")))
        later_nodes = {r["spec_node_id"] for r in cur.values()}
        for rid in sorted(set(prev) - set(cur)):
            rec = prev[rid]
            if not is_node_hash(rec["spec_node_id"]):
                continue
            if rec["spec_node_id"] not in later_nodes and rec["spec_node_id"] not in spec_nodes:
                lines.append(change_event(sha, rec, "removed", rec.get("spec_hash"), None))
        prev = cur

    for eid, task_id in created:
        status = tracker.get(task_id, (None, None, ""))[0]
        if status == "closed":
            lines.append({"event": "task_closed", "for": eid, "task_id": task_id})

    # Epic pairings. Map epic rows are authoritative; the title heuristic fills in
    # epics that predate the map's epic-row convention.
    stems = {p.stem for p in Path("spec/proposals").glob("2026-*.md")}
    for bead_id, (status, itype, title) in sorted(tracker.items()):
        if itype != "epic":
            continue
        matches = [s for s in stems if s in title]
        if len(matches) == 1 and matches[0] not in epic_pairs:
            epic_pairs[matches[0]] = bead_id
            print(f"epic pairing (title heuristic): {bead_id} ({title!r}) -> {matches[0]}")
        elif not matches:
            print(f"epic UNPAIRED: {bead_id} ({title!r}) — no stem match", file=sys.stderr)
    for slug in sorted(epic_pairs):
        task_id = epic_pairs[slug]
        lines.append({"event": "task_created", "proposal": slug, "task_id": task_id})
        if tracker.get(task_id, (None, None, ""))[0] == "closed":
            lines.append({"event": "task_closed", "proposal": slug, "task_id": task_id})
        print(f"epic pairing: {task_id} -> {slug}")

    JOURNAL.write_text("".join(json.dumps(l, separators=(",", ":")) + "\n" for l in lines))
    kinds = {}
    for l in lines:
        kinds[l["event"]] = kinds.get(l["event"], 0) + 1
    print(f"wrote {JOURNAL}: {len(lines)} lines over {len(versions)} map versions — {kinds}")


if __name__ == "__main__":
    main()
