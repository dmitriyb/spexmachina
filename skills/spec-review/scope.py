#!/usr/bin/env python3
"""Print the review scope of a spec change: seed nodes plus their graph neighbours.

usage: scope.py [--spec-dir DIR] [--spex BIN] [--hops N] [--free-preq] [--diff FILE] [--all] [--stage DIR] [--expand] [SEED ...]

A SEED is a 12-hex identity hash or a module name (every node of that module).
With no SEED and no --all, the seeds are the nodes `spex diff --json` reports
(read from --diff FILE when given, otherwise by running the command).

Two moves cost nothing: a node reaches its own module node, because the render
has no containment edge and `requires_module` neighbours must be reachable from
any changed node; and a module requirement reaches its project requirement,
because the parent is the authority the requirement is read against. Every
other declared edge costs one hop in both directions, including the downward
`preq_id` move from a project requirement to its other children. With
--free-preq that downward move is free too, so every sibling requirement under
a shared parent enters at the seed's hop; on a corpus whose project requirements
are widely shared that is most of the spec, which is why it is a flag.

A module reached through `requires_module<-` requires a module in scope: it is
a consumer of a changed contract, and its arch and flow leaves are where that
reliance is written, whether the leaf links, names or paraphrases the node it
relies on. Those leaves are printed as `seam` rows at the module's hop, so the
consumers of a change are read without waiting for a finding to reach them.

With --expand, the call is a walk step, not a change: seam and mention rows are
not derived (a change's dependents were staged by the initial call, and an
expansion seeded on a root module would otherwise stage the whole corpus), only
the seed's neighbours and the leaves it cites.

Rows labelled `mention` are leaves outside the scope that link a seed node's id
or name a seed component or api as a whole word: a reliance the graph does not
declare, visible only because the author named the node. A paraphrase is not
caught, and nothing mechanical catches it.

With --stage DIR, every file the scope names — project.json, each in-scope
module's module.json, each in-scope, seam and mention leaf — is copied into
DIR under its spec-relative path, and a `SCOPE.tsv` there accumulates the rows
across calls. Every module in scope is staged as `module.slim.json` — only its
requirements that are in scope, in full, and its components, apis, data flows
and test sections as id, name, description and reference fields — never the
full module.json: a module is read for the part of its contract the change
touches, and the changed descriptions are all in the slim file.
`FINDINGS.tsv` and `CELLS.tsv` are created with a header for the reviewer to
append to (see SKILL.md); the audit checks every quote in them against the
staged files. `UNDECLARED.tsv` carries the declared-versus-on-disk leaf check
over the whole spec, computed here so the reviewer never runs it by hand. With
--stage, stdout carries the summary only; the rows are in SCOPE.tsv.

`READ-PLAN.tsv` is the reading order: one row per tool call, the staged files
that call reads together, packed in a fixed order (project.json and the slim
module files first, then leaves in SCOPE order) under `read_call_bytes` from
`review.json` beside this script. Every reviewer follows it, so two reviewers'
reading phases are byte-identical and share the prompt cache, and no reviewer
spends turns deciding what to read. An expansion appends rows for the files it
adds. The summary also prints a `size` line — large when the seam reaches at
least `seam_modules_at_least` modules or the stage is at least
`staged_share_at_least` of the spec — and the `reviewers` count the config
declares for that size.
`PAIRS.tsv` lists every pair the judgement lenses must decide, both ends
staged: component × implemented requirement (with the project requirement
behind it), data flow × used component, seam leaf × changed contract, mention
leaf × seed node. The reviewer writes one verdict per row in its last column
before reporting, and the audit rejects a run with a row left blank. A
reviewer works from DIR and widens it by calling this script again with new
seeds and the same --stage.

Output, tab separated, one node per line, sorted by hop then module then type:
  hop  id  type  module  name  leaf  via  from
`via`/`from` name the edge and node that reached it (empty for a seed). The last
hop printed is `frontier`: the nodes one edge beyond --hops, reachable but not in
scope, so a truncated walk is visible rather than silent. A summary of modules in
scope and the frontier size goes to stderr.

Exit 0 with a scope, 1 on an input error, 2 when the seeds resolve to no node.
"""
import json, subprocess, sys, os, re

HEX = re.compile(r'^[a-f0-9]{12}$')

def die(msg, code=1):
    sys.stderr.write(f"scope.py: {msg}\n"); sys.exit(code)

def run(args):
    p = subprocess.run(args, capture_output=True, text=True)
    if p.returncode != 0:
        die(f"{' '.join(args)} exited {p.returncode}: {p.stderr.strip()}")
    return p.stdout

def main(argv):
    spec_dir, spex, hops, diff_file, all_nodes, seeds, free_preq, stage, expand = "spec", "bin/spex", 1, None, False, [], False, None, False
    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--spec-dir": spec_dir = argv[i+1]; i += 2
        elif a == "--spex": spex = argv[i+1]; i += 2
        elif a == "--hops": hops = int(argv[i+1]); i += 2
        elif a == "--diff": diff_file = argv[i+1]; i += 2
        elif a == "--all": all_nodes = True; i += 1
        elif a == "--free-preq": free_preq = True; i += 1
        elif a == "--stage": stage = argv[i+1]; i += 2
        elif a == "--expand": expand = True; i += 1
        elif a in ("-h", "--help"): print(__doc__); return 0
        elif a.startswith("-"): die(f"unknown flag {a}")
        else: seeds.append(a); i += 1
    spec_dir = spec_dir.rstrip("/")

    graph = json.loads(run([spex, "--spec-dir", spec_dir, "render", "--format", "json"]))
    project = json.load(open(os.path.join(spec_dir, "project.json")))
    mod_by_hash = {m["id"]: m["name"] for m in project["modules"]}
    mod_path = {m["name"]: m.get("path", m["name"]) for m in project["modules"]}

    # bare key -> node record; leaf paths from the module files
    nodes, bare_of = {}, {}
    for n in graph["nodes"]:
        if n["type"] == "project": continue
        bare = n["name"] if n["type"] == "module" else n["id"].rsplit(":", 1)[-1]
        rec = {"id": bare, "rid": n["id"], "type": n["type"], "module": n.get("module") or ("-" if n["type"] != "module" else n["name"]), "name": n["name"], "leaf": ""}
        nodes[bare] = rec; bare_of[n["id"]] = bare
    for mname, mpath in mod_path.items():
        mj = os.path.join(spec_dir, mpath, "module.json")
        if not os.path.exists(mj): continue
        m = json.load(open(mj))
        for arr in m:
            if isinstance(m[arr], list):
                for e in m[arr]:
                    if isinstance(e, dict) and e.get("id") in nodes and e.get("content"):
                        nodes[e["id"]]["leaf"] = f"{mpath}/{e['content']}"
        if mname in nodes: nodes[mname]["leaf"] = f"{mpath}/module.json"

    adj = {}  # node -> [(neighbour, edge label, cost)]
    for e in graph["edges"]:
        a, b = bare_of.get(e["from"]), bare_of.get(e["to"])
        if not a or not b: continue
        up = 0 if e["type"] == "preq_id" else 1
        down = 0 if (free_preq and e["type"] == "preq_id") else 1
        adj.setdefault(a, []).append((b, e["type"] + "->", up))
        adj.setdefault(b, []).append((a, e["type"] + "<-", down))
    for k, n in nodes.items():
        if n["type"] != "module" and n["module"] in nodes:
            adj.setdefault(k, []).append((n["module"], "in-module", 0))

    seed_set = set()
    if all_nodes:
        seed_set = set(nodes)
    elif seeds:
        for s in seeds:
            if HEX.match(s) and s in nodes: seed_set.add(s)
            elif s in mod_path: seed_set.update(k for k, v in nodes.items() if v["module"] == s or (v["type"] == "module" and v["name"] == s))
            else: die(f"seed {s!r} is neither a known identity hash nor a module name")
    else:
        d = json.load(open(diff_file)) if diff_file else json.loads(run([spex, "--spec-dir", spec_dir, "diff", "--json"]))
        for c in d.get("changes", []):
            p = c["path"]
            if p.startswith("meta/"):
                h = p.split("/", 1)[1]
                if h in mod_by_hash: seed_set.add(mod_by_hash[h])
            elif p in nodes: seed_set.add(p)
    if not seed_set: die("no seed resolved to a node", 2)

    # 0/1 BFS to the frontier depth: zero-cost edges relax within a hop
    from collections import deque
    hop = {s: 0 for s in seed_set}; via = {s: ("", "") for s in seed_set}
    dq = deque(sorted(seed_set))
    while dq:
        a = dq.popleft()
        for b, label, cost in adj.get(a, []):
            h = hop[a] + cost
            if h > hops + 1: continue
            if b not in hop or h < hop[b]:
                hop[b] = h; via[b] = (label, a)
                if cost == 0: dq.appendleft(b)
                else: dq.append(b)

    # seam rows: the arch and flow leaves of every module that requires a module in scope
    seam = {}
    for k in ([] if expand else list(hop)):
        n = nodes[k]
        if n["type"] == "module" and hop[k] <= hops and via[k][0] == "requires_module<-":
            for j, m in nodes.items():
                if m["module"] == n["name"] and m["type"] in ("component", "data_flow") and j not in hop:
                    seam[j] = (hop[k], k)
    order = {"module": 0, "requirement": 1, "component": 2, "data_flow": 3, "test_section": 4, "api": 5}
    rows = sorted(hop, key=lambda k: (hop[k], nodes[k]["module"], order.get(nodes[k]["type"], 9), nodes[k]["name"]))
    quiet = bool(stage)  # when staging, the rows go to SCOPE.tsv and stdout carries the summary only
    if not quiet: print("hop\tid\ttype\tmodule\tname\tleaf\tvia\tfrom")
    in_scope = set()
    for k in rows:
        n = nodes[k]; hh = hop[k]
        label = "frontier" if hh > hops else str(hh)
        if hh <= hops: in_scope.add(n["module"] if n["type"] != "module" else n["name"])
        if not quiet: print("\t".join([label, k, n["type"], n["module"], n["name"], n["leaf"], via[k][0], via[k][1]]))
    seam_rows = []
    for k in sorted(seam, key=lambda k: (seam[k][0], nodes[k]["module"], nodes[k]["name"])):
        n = nodes[k]
        seam_rows.append(["seam", k, n["type"], n["module"], n["name"], n["leaf"], "requires", seam[k][1]])
        if not quiet: print("\t".join(seam_rows[-1]))
    # mention rows: leaves outside scope and seam that link a seed id or name a seed component/api
    seed_names = {nodes[k]["name"] for k in seed_set if nodes[k]["type"] in ("component", "api")}
    name_re = re.compile(r"(?<![A-Za-z0-9_])(" + "|".join(re.escape(x) for x in sorted(seed_names, key=len, reverse=True)) + r")(?![A-Za-z0-9_])") if seed_names else None
    covered = {k for k in hop if hop[k] <= hops} | set(seam)
    mention_rows = []
    for k, n in ([] if expand else nodes.items()):
        if k in covered or n["type"] not in ("component", "data_flow") or not n["leaf"]: continue
        path = os.path.join(spec_dir, n["leaf"])
        if not os.path.exists(path): continue
        text = open(path, encoding="utf-8", errors="replace").read()
        hit = next((sid for sid in sorted(seed_set) if f"[[{sid}|" in text or f'[["{sid}"|' in text), None)
        if not hit and name_re:
            m = name_re.search(text); hit = m.group(1) if m else None
        if hit:
            mention_rows.append(["mention", k, n["type"], n["module"], n["name"], n["leaf"], "names", hit])
    if not quiet:
        for r in sorted(mention_rows, key=lambda r: (r[3], r[4])): print("\t".join(r))
    # cites rows: the reverse of mention — a node outside the scope that a seed leaf links or names.
    # A claim a seed makes about another node is read because the seed made it, not because a
    # reviewer happened to chase it; this is what makes expansion deterministic for the first hop.
    named = {n["name"]: k for k, n in nodes.items() if n["type"] in ("component", "api")}
    name_all = re.compile(r"(?<![A-Za-z0-9_])(" + "|".join(re.escape(x) for x in sorted(named, key=len, reverse=True)) + r")(?![A-Za-z0-9_])") if named else None
    covered_all = covered | set(seam) | {r[1] for r in mention_rows}
    cites_rows, cited = [], set()
    for k in sorted(seed_set):
        n = nodes[k]
        if not n["leaf"] or n["type"] == "module": continue
        path = os.path.join(spec_dir, n["leaf"])
        if not os.path.exists(path): continue
        text = open(path, encoding="utf-8", errors="replace").read()
        targets = set(re.findall(r"\[\[\"?([a-f0-9]{12})\"?\|", text))
        if name_all: targets |= {named[m] for m in name_all.findall(text)}
        for t in sorted(targets):
            if t in covered_all or t in cited or t not in nodes or not nodes[t]["leaf"] or nodes[t]["type"] == "module": continue
            cited.add(t); tn = nodes[t]
            cites_rows.append(["cites", t, tn["type"], tn["module"], tn["name"], tn["leaf"], "cited-by", k])
    if not quiet:
        for r in cites_rows: print("\t".join(r))
    fr = [k for k in rows if hop[k] > hops]
    sys.stderr.write(f"scope: {sum(1 for k in rows if hop[k] <= hops)} nodes in {len(in_scope)} modules ({', '.join(sorted(in_scope))}); {len(seam)} seam leaves in dependent modules; {len(mention_rows)} mention leaves; {len(cites_rows)} cited leaves; frontier {len(fr)} nodes beyond hop {hops}\n")
    if stage:
        import shutil
        os.makedirs(stage, exist_ok=True)
        cfg_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "review.json")
        cfg = json.load(open(cfg_path)) if os.path.exists(cfg_path) else {}
        files = {"project.json"}
        slim_modules = set()
        for k in sorted(covered):
            n = nodes[k]
            if n["leaf"] and n["type"] != "module": files.add(n["leaf"])
            mname = n["name"] if n["type"] == "module" else n["module"]
            if mname in mod_path: slim_modules.add(mname)
        for r in mention_rows: files.add(r[5])
        for r in cites_rows: files.add(r[5])
        for f in sorted(files):
            src, dst = os.path.join(spec_dir, f), os.path.join(stage, f)
            if os.path.exists(src):
                os.makedirs(os.path.dirname(dst), exist_ok=True); shutil.copyfile(src, dst)
        for mname in sorted(slim_modules):
            src = os.path.join(spec_dir, mod_path[mname], "module.json")
            if not os.path.exists(src): continue
            m = json.load(open(src))
            slim = {"name": m.get("name"), "description": m.get("description"), "requirements": [r for r in m.get("requirements", []) if r.get("id") in covered]}
            for arr in ("components", "apis", "data_flows", "test_sections"):
                if arr in m:
                    slim[arr] = [{k: e.get(k) for k in ("id", "name", "description", "implements", "uses", "provided_by", "describes") if e.get(k) is not None} for e in m[arr]]
            dst = os.path.join(stage, mod_path[mname], "module.slim.json")
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            if os.path.exists(dst):  # a later call widens the slim file, never narrows it: union by id
                old = json.load(open(dst, encoding="utf-8"))
                for arr in ("requirements", "components", "apis", "data_flows", "test_sections"):
                    have = {e["id"] for e in slim.get(arr, [])}
                    slim[arr] = slim.get(arr, []) + [e for e in old.get(arr, []) if e.get("id") not in have]
            json.dump(slim, open(dst, "w", encoding="utf-8"), indent=1, ensure_ascii=False)
            files.add(f"{mod_path[mname]}/module.slim.json")
        # PAIRS.tsv: every pair a judgement lens must decide, both ends staged
        pairs = []
        leaf_of = lambda k: nodes[k]["leaf"] if k in nodes else ""
        for k in sorted(covered):
            n = nodes[k]
            if n["type"] == "component":
                for b, label, _ in adj.get(k, []):
                    if label == "implements->":
                        preq = next((c for c, l2, _ in adj.get(b, []) if l2 == "preq_id->"), "")
                        pairs.append(["lens8", k, n["name"], leaf_of(k), b, nodes[b]["name"], preq])
            if n["type"] == "data_flow":
                for b, label, _ in adj.get(k, []):
                    if label == "uses->" and (b in covered or b in seam or b in cited): pairs.append(["lens3", k, n["name"], leaf_of(k), b, nodes[b]["name"], leaf_of(b)])
        seed_contracts = ",".join(sorted(k for k in seed_set if nodes[k]["type"] in ("component", "api", "data_flow")))
        for r in seam_rows: pairs.append(["seam", r[1], r[4], r[5], "changed-contracts", seed_contracts, ""])
        for r in mention_rows: pairs.append(["mention", r[1], r[4], r[5], r[7], "", ""])
        for r in cites_rows: pairs.append(["cites", r[7], nodes[r[7]]["name"], leaf_of(r[7]), r[1], r[4], r[5]])
        for name, header in (("FINDINGS.tsv", "id\tleft_id\tleft_file\tleft_quote\tright_id\tright_file\tright_quote\tcontradiction\tfix\n"),
                             ("CELLS.tsv", "leaf\tmode_a\tmode_b\tdecided_by_quote\tfinding\n")):
            fp = os.path.join(stage, name)
            if not os.path.exists(fp): open(fp, "w").write(header)
        # candidate cells: every pair of flags a staged arch leaf names; the reviewer decides each
        cpath = os.path.join(stage, "CELLS.tsv")
        have = {tuple(l.rstrip("\n").split("\t")[:3]) for l in open(cpath) if not l.startswith("leaf\t")}
        with open(cpath, "a") as out:
            for f in sorted(files):
                if not (f.endswith(".md") and os.path.basename(f).startswith(("arch_", "flow_"))): continue
                text = open(os.path.join(stage, f), encoding="utf-8", errors="replace").read()
                flags = sorted(set(re.findall(r"(?<![A-Za-z0-9-])(" + cfg.get("flag_pattern", "--[a-z][a-z0-9-]+") + r")", text)))
                for i in range(len(flags)):
                    for j in range(i + 1, len(flags)):
                        if (f, flags[i], flags[j]) not in have: out.write(f"{f}\t{flags[i]}\t{flags[j]}\t\t\n"); have.add((f, flags[i], flags[j]))
        sys.stderr.write(f"cells: {len(have)} candidate flag pairs in CELLS.tsv\n")
        ppath = os.path.join(stage, "PAIRS.tsv")
        existing = set()
        if os.path.exists(ppath):
            existing = {tuple(l.rstrip("\n").split("\t")[:5]) for l in open(ppath) if not l.startswith("kind\t")}
        with open(ppath, "a") as out:
            if not existing: out.write("kind\tleft_id\tleft_name\tleft_leaf\tright_id\tright_name\tright_ref\tverdict\n")
            for p in pairs:
                if tuple(p[:5]) not in existing: out.write("\t".join(p) + "\t\n")
        # the undeclared-leaf check, so the reviewer never improvises it over the spec
        declared = set()
        for mname, mpath in mod_path.items():
            mj = os.path.join(spec_dir, mpath, "module.json")
            if not os.path.exists(mj): continue
            for arr in json.load(open(mj)).values():
                if isinstance(arr, list):
                    for e in arr:
                        if isinstance(e, dict) and e.get("content"): declared.add(f"{mpath}/{e['content']}")
        ondisk = set()
        for root, _, fs in os.walk(spec_dir):
            if "proposals" in root.split(os.sep): continue
            for f in fs:
                if f.endswith(".md"): ondisk.add(os.path.relpath(os.path.join(root, f), spec_dir))
        undeclared = sorted(ondisk - declared); missing = sorted(declared - ondisk)
        with open(os.path.join(stage, "UNDECLARED.tsv"), "w") as out:
            for u in undeclared: out.write(f"undeclared\t{u}\n")
            for m in missing: out.write(f"missing\t{m}\n")
        sys.stderr.write(f"leaves: {len(undeclared)} on disk but undeclared, {len(missing)} declared but missing (UNDECLARED.tsv)\n")
        with open(os.path.join(stage, "SCOPE.tsv"), "a") as out:
            for k in rows:  # frontier rows are written too, so the reviewer and the tests can name what lies one edge beyond
                n = nodes[k]; hh = hop[k]
                out.write("\t".join(["frontier" if hh > hops else str(hh), k, n["type"], n["module"], n["name"], n["leaf"], via[k][0], via[k][1]]) + "\n")
            for r in seam_rows + mention_rows + cites_rows: out.write("\t".join(r) + "\n")
        sys.stderr.write(f"stage: {len(files)} files under {stage}; modules slim {len(slim_modules)}; pairs {len(pairs)}\n")
        # read plan: fixed order, packed under the call limit; only files not already planned
        limit = int(cfg.get("read_call_bytes", 24000))
        plan_path = os.path.join(stage, "READ-PLAN.tsv")
        planned, last_call = set(), 0
        if os.path.exists(plan_path):
            for l in open(plan_path):
                c = l.rstrip("\n").split("\t")
                if c[0].isdigit(): last_call = max(last_call, int(c[0])); planned.update(c[1].split(","))
        ordered = ["project.json"] + [f"{mod_path[m]}/module.slim.json" for m in sorted(slim_modules)]
        seen_leaf = set()
        for k in rows:
            n = nodes[k]
            if hop[k] <= hops and n["leaf"] and n["type"] != "module" and n["leaf"] not in seen_leaf: ordered.append(n["leaf"]); seen_leaf.add(n["leaf"])
        for r in seam_rows + mention_rows + cites_rows:
            if r[5] not in seen_leaf: ordered.append(r[5]); seen_leaf.add(r[5])
        calls, cur, cur_size = [], [], 0
        for f in ordered:
            if f in planned: continue
            p = os.path.join(stage, f)
            if not os.path.exists(p): continue
            sz = os.path.getsize(p)
            if cur and cur_size + sz > limit: calls.append(cur); cur, cur_size = [], 0
            cur.append(f); cur_size += sz
        if cur: calls.append(cur)
        with open(plan_path, "a") as out:
            if last_call == 0: out.write("call\tfiles\tbytes\n")
            for i, c in enumerate(calls, start=last_call + 1):
                out.write(f"{i}\t{','.join(c)}\t{sum(os.path.getsize(os.path.join(stage, f)) for f in c)}\n")
        # size line and reviewers
        spec_bytes = sum(os.path.getsize(os.path.join(r_, f)) for r_, _, fs in os.walk(spec_dir) if "proposals" not in r_.split(os.sep) for f in fs if f.endswith((".md", ".json")))
        stage_bytes = sum(os.path.getsize(os.path.join(r_, f)) for r_, _, fs in os.walk(stage) for f in fs if f.endswith((".md", ".json")))
        seam_mods = {nodes[k]["module"] for k in seam}
        lw = cfg.get("large_when", {})
        large = len(seam_mods) >= int(lw.get("seam_modules_at_least", 3)) or (spec_bytes and stage_bytes / spec_bytes >= float(lw.get("staged_share_at_least", 0.30)))
        size = "large" if large else "small"
        sys.stderr.write(f"read plan: {len(calls)} calls appended (READ-PLAN.tsv, limit {limit} bytes)\n")
        sys.stderr.write(f"size: {size} (seam modules {len(seam_mods)}, staged share {stage_bytes / spec_bytes if spec_bytes else 0:.0%}); reviewers: {cfg.get('reviewers', {}).get(size, 1)}\n")
    return 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
