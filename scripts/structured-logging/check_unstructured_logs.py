#!/usr/bin/env python3
"""Fail when production code adds unstructured logging.

Existing call sites are listed in baseline.txt. A run fails only when a
scoped file contains a call the baseline does not already allow. Deleting a
call does not fail, so other lanes can migrate without editing this file.

Go scope is pkg/ except the allowlist: fmt.Print/Printf/Println and calls on
the standard library log package. TypeScript scope is public/app: console
logging methods. Tests, stories, fixtures, generated files, and a few copied
libraries are listed in allowlist.txt.
"""

from __future__ import annotations

import argparse
import re
import sys
import tempfile
import unittest
from collections import Counter
from pathlib import Path

KIND_TS_CONSOLE = "ts-console"
KIND_GO_FMT = "go-fmt-print"
KIND_GO_LOG = "go-stdlib-log"

TS_ROOT = "public/app"
GO_ROOT = "pkg"
TS_SUFFIXES = {".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}

CONSOLE_RE = re.compile(
    r"\bconsole\s*(?:\?\s*)?\.\s*(?:debug|error|info|log|trace|warn|dir|table|assert|count|"
    r"timeEnd|timeLog|time|groupCollapsed|groupEnd|group)\b"
)
FMT_RE = re.compile(r"\bfmt\.(?:Println|Printf|Print)\(")
STDLIB_FUNCS = (
    "Fatalf|Fatalln|Fatal|Panicf|Panicln|Panic|Printf|Println|Print|New"
)
IMPORT_LINE_RE = re.compile(
    r'^import\s+(?:([A-Za-z_][A-Za-z0-9_]*|\.|_)\s+)?"([^"]+)"\s*$'
)
IMPORT_SPEC_RE = re.compile(
    r'^(?:([A-Za-z_][A-Za-z0-9_]*|\.|_)\s+)?"([^"]+)"$'
)

ViolationKey = tuple[str, str, str]  # kind, path, normalized line


def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def script_dir() -> Path:
    return Path(__file__).resolve().parent


def normalize(line: str) -> str:
    return " ".join(line.split())


def load_allowlist(path: Path) -> list[tuple[str, str]]:
    rules: list[tuple[str, str]] = []
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.split("#", 1)[0].strip()
        if not line:
            continue
        kind, value = line.split(":", 1)
        rules.append((kind.strip(), value.strip()))
    return rules


def is_allowed(rel: str, rules: list[tuple[str, str]]) -> bool:
    parts = rel.split("/")
    name = parts[-1]
    for kind, value in rules:
        if kind == "exact" and rel == value:
            return True
        if kind == "prefix":
            prefix = value.rstrip("/")
            if rel == prefix or rel.startswith(prefix + "/"):
                return True
        if kind == "suffix" and name.endswith(value):
            return True
        if kind == "segment" and value in parts[:-1]:
            return True
        if kind == "path-contains" and value in rel:
            return True
    return False


def is_generated(text: str) -> bool:
    head = "\n".join(text.splitlines()[:40])
    return "Code generated" in head and "DO NOT EDIT" in head


def split_code(src: str, lang: str) -> tuple[str, str]:
    """Return (match_text, key_text), both the same length as src.

    match_text blanks comments and string contents so searches only see code.
    Template expressions (${...}) stay visible. key_text blanks comments only,
    so the baseline line still includes the call's arguments.
    """
    match: list[str] = []
    key: list[str] = []
    i = 0
    n = len(src)

    while i < n:
        ch = src[i]
        nxt = src[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            while i < n and src[i] != "\n":
                match.append(" ")
                key.append(" ")
                i += 1
            continue
        if ch == "/" and nxt == "*":
            match.append(" ")
            key.append(" ")
            match.append(" ")
            key.append(" ")
            i += 2
            while i < n and not (src[i] == "*" and i + 1 < n and src[i + 1] == "/"):
                blank = "\n" if src[i] == "\n" else " "
                match.append(blank)
                key.append(blank)
                i += 1
            if i < n:
                match.append(" ")
                key.append(" ")
                match.append(" ")
                key.append(" ")
                i += 2
            continue
        if ch in ('"', "'", "`"):
            i = _copy_string(src, i, lang, match, key)
            continue
        match.append(ch)
        key.append(ch)
        i += 1

    match_text = "".join(match)
    key_text = "".join(key)
    if len(match_text) != n or len(key_text) != n:
        raise RuntimeError("split_code changed the source length")
    return match_text, key_text


def _copy_string(src: str, i: int, lang: str, match: list[str], key: list[str]) -> int:
    quote = src[i]
    n = len(src)
    match.append(" ")
    key.append(quote)
    i += 1

    if quote == "`" and lang == "go":
        while i < n:
            ch = src[i]
            if ch == "`":
                match.append(" ")
                key.append("`")
                return i + 1
            match.append("\n" if ch == "\n" else " ")
            key.append(ch)
            i += 1
        return i

    while i < n:
        ch = src[i]
        if lang == "ts" and quote == "`" and ch == "$" and i + 1 < n and src[i + 1] == "{":
            match.append("$")
            key.append("$")
            match.append("{")
            key.append("{")
            i += 2
            i = _copy_template_expr(src, i, lang, match, key)
            continue
        if ch == "\\" and (quote != "`" or lang == "ts"):
            match.append(" ")
            key.append(ch)
            i += 1
            if i < n:
                match.append("\n" if src[i] == "\n" else " ")
                key.append(src[i])
                i += 1
            continue
        if ch == quote:
            match.append(" ")
            key.append(quote)
            return i + 1
        match.append("\n" if ch == "\n" else " ")
        key.append(ch)
        i += 1
    return i


def _copy_template_expr(
    src: str, i: int, lang: str, match: list[str], key: list[str]
) -> int:
    """Copy a ${...} expression body. i points just after '${'."""
    start = i
    end = _find_template_expr_end(src, i)
    expr_match, expr_key = split_code(src[start:end], lang)
    match.extend(expr_match)
    key.extend(expr_key)
    if end < len(src) and src[end] == "}":
        match.append("}")
        key.append("}")
        return end + 1
    return end


def _find_template_expr_end(src: str, i: int) -> int:
    depth = 1
    n = len(src)
    while i < n and depth:
        ch = src[i]
        nxt = src[i + 1] if i + 1 < n else ""
        if ch == "/" and nxt == "/":
            i += 2
            while i < n and src[i] != "\n":
                i += 1
            continue
        if ch == "/" and nxt == "*":
            i += 2
            while i < n and not (src[i] == "*" and i + 1 < n and src[i + 1] == "/"):
                i += 1
            i = min(n, i + 2)
            continue
        if ch in ('"', "'", "`"):
            i = _skip_string_end(src, i)
            continue
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return i
        i += 1
    return i


def _skip_string_end(src: str, i: int) -> int:
    quote = src[i]
    i += 1
    n = len(src)
    while i < n:
        ch = src[i]
        if ch == "\\":
            i += 2
            continue
        if ch == quote:
            return i + 1
        i += 1
    return i


def line_spans(text: str) -> list[tuple[int, int]]:
    spans: list[tuple[int, int]] = []
    start = 0
    for idx, ch in enumerate(text):
        if ch == "\n":
            spans.append((start, idx))
            start = idx + 1
    spans.append((start, len(text)))
    return spans


def line_index_at(spans: list[tuple[int, int]], offset: int) -> int:
    for idx, (start, end) in enumerate(spans):
        if start <= offset <= end:
            return idx
    return len(spans) - 1


def stdlib_log_aliases(key_text: str) -> list[tuple[str, int]]:
    """Return (local name, line index) for imports of the standard library log package."""
    found: list[tuple[str, int]] = []
    lines = key_text.splitlines()
    i = 0
    while i < len(lines):
        stripped = lines[i].strip()
        single = IMPORT_LINE_RE.match(stripped)
        if single:
            if single.group(2) == "log":
                found.append((single.group(1) or "log", i))
            i += 1
            continue
        if stripped == "import (":
            i += 1
            while i < len(lines) and lines[i].strip() != ")":
                spec = IMPORT_SPEC_RE.match(lines[i].strip())
                if spec and spec.group(2) == "log":
                    found.append((spec.group(1) or "log", i))
                i += 1
            i += 1
            continue
        i += 1
    return found


def add_line(
    counts: Counter[ViolationKey],
    kind: str,
    rel: str,
    key_text: str,
    line_no: int,
) -> None:
    lines = key_text.splitlines()
    if line_no >= len(lines):
        return
    text = normalize(lines[line_no])
    if not text:
        return
    counts[(kind, rel, text)] += 1


def scan_ts(rel: str, text: str, counts: Counter[ViolationKey]) -> None:
    match_text, key_text = split_code(text, "ts")
    spans = line_spans(match_text)
    for match in CONSOLE_RE.finditer(match_text):
        add_line(counts, KIND_TS_CONSOLE, rel, key_text, line_index_at(spans, match.start()))


def scan_go(rel: str, text: str, counts: Counter[ViolationKey]) -> None:
    match_text, key_text = split_code(text, "go")
    spans = line_spans(match_text)
    for match in FMT_RE.finditer(match_text):
        add_line(counts, KIND_GO_FMT, rel, key_text, line_index_at(spans, match.start()))

    for alias, line_no in stdlib_log_aliases(key_text):
        if alias in {".", "_"}:
            add_line(counts, KIND_GO_LOG, rel, key_text, line_no)
            continue
        call_re = re.compile(rf"\b{re.escape(alias)}\.(?:{STDLIB_FUNCS})\b")
        for match in call_re.finditer(match_text):
            add_line(counts, KIND_GO_LOG, rel, key_text, line_index_at(spans, match.start()))


def scan_tree(root: Path, rules: list[tuple[str, str]]) -> Counter[ViolationKey]:
    counts: Counter[ViolationKey] = Counter()
    for top, suffixes, scanner in (
        (TS_ROOT, TS_SUFFIXES, scan_ts),
        (GO_ROOT, {".go"}, scan_go),
    ):
        base = root / top
        if not base.is_dir():
            continue
        for path in base.rglob("*"):
            if not path.is_file() or path.suffix not in suffixes:
                continue
            if "node_modules" in path.parts:
                continue
            rel = path.relative_to(root).as_posix()
            if is_allowed(rel, rules):
                continue
            text = path.read_text(encoding="utf-8", errors="replace")
            if is_generated(text):
                continue
            scanner(rel, text, counts)
    return counts


def format_baseline(counts: Counter[ViolationKey]) -> str:
    lines = [
        "# Existing unstructured-logging call sites.",
        "# The check fails when scoped code contains a call that is not listed here.",
        "# Columns: count, kind, path, normalized source line.",
        "# Refresh after calls are removed:",
        "#   python3 scripts/structured-logging/check_unstructured_logs.py --update",
    ]
    for kind, path, text in sorted(counts):
        lines.append(f"{counts[(kind, path, text)]}\t{kind}\t{path}\t{text}")
    lines.append("")
    return "\n".join(lines)


def load_baseline(path: Path) -> Counter[ViolationKey]:
    counts: Counter[ViolationKey] = Counter()
    if not path.is_file():
        raise FileNotFoundError(f"baseline not found: {path}")
    for raw in path.read_text(encoding="utf-8").splitlines():
        if not raw.strip() or raw.startswith("#"):
            continue
        count_s, kind, rel, text = raw.split("\t", 3)
        counts[(kind, rel, text)] += int(count_s)
    return counts


def compare(
    current: Counter[ViolationKey], baseline: Counter[ViolationKey]
) -> tuple[list[tuple[ViolationKey, int]], list[tuple[ViolationKey, int]]]:
    new: list[tuple[ViolationKey, int]] = []
    stale: list[tuple[ViolationKey, int]] = []
    for key, count in sorted(current.items()):
        extra = count - baseline.get(key, 0)
        if extra > 0:
            new.append((key, extra))
    for key, count in sorted(baseline.items()):
        missing = count - current.get(key, 0)
        if missing > 0:
            stale.append((key, missing))
    return new, stale


def report(new: list[tuple[ViolationKey, int]], stale: list[tuple[ViolationKey, int]]) -> int:
    if stale:
        preview = stale[:20]
        print(
            f"note: {sum(n for _, n in stale)} baselined call(s) are gone. "
            "That does not fail the check. Refresh the baseline with --update "
            "after migrations so those calls cannot be reintroduced unnoticed.",
            file=sys.stderr,
        )
        for (kind, path, text), missing in preview:
            print(f"  stale {missing} {kind} {path} {text}", file=sys.stderr)
        if len(stale) > len(preview):
            print(f"  ... and {len(stale) - len(preview)} more", file=sys.stderr)

    if not new:
        print("unstructured logging check passed")
        return 0

    total = sum(n for _, n in new)
    print(
        f"unstructured logging check failed: {total} new call(s) not in the baseline",
        file=sys.stderr,
    )
    for (kind, path, text), extra in new[:50]:
        print(f"  +{extra} {kind} {path}: {text}", file=sys.stderr)
    if len(new) > 50:
        print(f"  ... and {len(new) - 50} more", file=sys.stderr)
    print(
        "Use pkg/infra/log in Go and the frontend structured logger in public/app. "
        "Do not add a baseline row to permit a new call.",
        file=sys.stderr,
    )
    return 1


def run_check(root: Path, baseline_path: Path, allowlist_path: Path, update: bool) -> int:
    rules = load_allowlist(allowlist_path)
    current = scan_tree(root, rules)
    if update:
        baseline_path.write_text(format_baseline(current), encoding="utf-8")
        print(f"wrote {len(current)} baseline entries to {baseline_path}")
        return 0
    baseline = load_baseline(baseline_path)
    new, stale = compare(current, baseline)
    return report(new, stale)


class CheckerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.allowlist = script_dir() / "allowlist.txt"

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def write(self, rel: str, text: str) -> None:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def scan(self) -> Counter[ViolationKey]:
        return scan_tree(self.root, load_allowlist(self.allowlist))

    def test_split_code_keeps_length_and_hides_comments(self) -> None:
        src = 'console.log("http://x") // console.log("no")\n/* console.warn(1) */\n'
        match_text, key_text = split_code(src, "ts")
        self.assertEqual(len(match_text), len(src))
        self.assertEqual(len(key_text), len(src))
        self.assertEqual(len(CONSOLE_RE.findall(match_text)), 1)
        self.assertIn("http://x", key_text)

    def test_template_expression_is_visible(self) -> None:
        src = "const s = `no ${console.log('yes')}`\n"
        match_text, _ = split_code(src, "ts")
        self.assertEqual(CONSOLE_RE.findall(match_text), ["console.log"])

    def test_optional_chaining_is_a_call(self) -> None:
        self.write("public/app/maybe.ts", 'console?.warn("x")\n')
        counts = self.scan()
        self.assertEqual(len(counts), 1)
        self.assertEqual(next(iter(counts))[0], KIND_TS_CONSOLE)

    def test_string_literal_is_not_a_call(self) -> None:
        self.write("public/app/string.ts", 'const s = "console.log(\'no\')"\n')
        self.assertEqual(self.scan(), Counter())

    def test_new_console_call_is_outside_baseline(self) -> None:
        self.write("public/app/old.ts", 'console.log("keep")\n')
        baseline = self.scan()
        self.write("public/app/new.ts", 'console.error("new")\n')
        new, _ = compare(self.scan(), baseline)
        self.assertEqual(len(new), 1)
        self.assertEqual(new[0][0][0], KIND_TS_CONSOLE)
        self.assertEqual(new[0][0][1], "public/app/new.ts")

    def test_tests_mocks_and_comments_are_ignored(self) -> None:
        self.write("public/app/widget.test.ts", 'console.log("test")\n')
        self.write("public/app/feature/mocks/data.ts", 'console.log("mock")\n')
        self.write("public/app/note.ts", '// console.log("comment")\n')
        self.assertEqual(self.scan(), Counter())

    def test_go_print_and_stdlib_log(self) -> None:
        self.write("pkg/svc/print.go", 'package svc\nimport "fmt"\nfunc f(){ fmt.Println("a") }\n')
        self.write(
            "pkg/svc/std.go",
            'package svc\nimport stdlog "log"\nfunc f(){ stdlog.Printf("b") }\n',
        )
        self.write(
            "pkg/svc/format.go",
            'package svc\nimport "fmt"\nfunc f(){ _ = fmt.Sprintf("fmt.Println") }\n',
        )
        self.write("pkg/svc/print_test.go", 'package svc\nimport "fmt"\nfunc TestF(){ fmt.Println("t") }\n')
        self.write("pkg/infra/log/log.go", 'package log\nimport "log"\nfunc f(){ log.Printf("impl") }\n')
        counts = self.scan()
        kinds = {(kind, path) for kind, path, _ in counts}
        self.assertIn((KIND_GO_FMT, "pkg/svc/print.go"), kinds)
        self.assertIn((KIND_GO_LOG, "pkg/svc/std.go"), kinds)
        self.assertNotIn((KIND_GO_FMT, "pkg/svc/format.go"), kinds)
        self.assertNotIn((KIND_GO_FMT, "pkg/svc/print_test.go"), kinds)
        self.assertFalse(any(path.startswith("pkg/infra/log/") for _, path, _ in counts))

    def test_removed_call_is_not_a_failure(self) -> None:
        self.write("pkg/svc/print.go", 'package svc\nimport "fmt"\nfunc f(){ fmt.Println("a") }\n')
        baseline = self.scan()
        self.write("pkg/svc/print.go", "package svc\nfunc f(){}\n")
        new, stale = compare(self.scan(), baseline)
        self.assertEqual(new, [])
        self.assertEqual(len(stale), 1)

    def test_duplicate_call_fails(self) -> None:
        self.write("pkg/svc/print.go", 'package svc\nimport "fmt"\nfunc f(){\nfmt.Println("a")\n}\n')
        baseline = self.scan()
        self.write(
            "pkg/svc/print.go",
            'package svc\nimport "fmt"\nfunc f(){\nfmt.Println("a")\nfmt.Println("a")\n}\n',
        )
        new, _ = compare(self.scan(), baseline)
        self.assertEqual(new[0][1], 1)

    def test_grafana_logger_import_is_not_stdlib(self) -> None:
        self.write(
            "pkg/svc/wrap.go",
            'package svc\nimport glog "github.com/grafana/grafana/pkg/infra/log"\n'
            "func f(){ glog.New(\"svc\") }\n",
        )
        self.assertEqual(self.scan(), Counter())

    def test_dev_fixture_is_allowlisted(self) -> None:
        self.write(
            "pkg/services/live/pipeline/devdata.go",
            'package pipeline\nimport "log"\nfunc f(){ log.Println("dev") }\n',
        )
        self.assertEqual(self.scan(), Counter())


def self_test() -> int:
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(CheckerTests)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    return 0 if result.wasSuccessful() else 1


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true", help="run the checker unit tests")
    parser.add_argument("--update", action="store_true", help="rewrite the baseline from the tree")
    parser.add_argument("--root", type=Path, default=None)
    parser.add_argument("--baseline", type=Path, default=None)
    parser.add_argument("--allowlist", type=Path, default=None)
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.self_test:
        return self_test()
    root = (args.root or repo_root()).resolve()
    baseline = (args.baseline or (script_dir() / "baseline.txt")).resolve()
    allowlist = (args.allowlist or (script_dir() / "allowlist.txt")).resolve()
    return run_check(root, baseline, allowlist, args.update)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
