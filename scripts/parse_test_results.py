#!/usr/bin/env python3
"""Parse `go test -json` output and generate a markdown report."""
import json
import re
import sys
from datetime import date
from pathlib import Path

CATEGORIES = [
    ("Authentication",        range(1,  7)),
    ("JWT Middleware",         range(7,  10)),
    ("Project CRUD",          range(10, 17)),
    ("Project Lifecycle",     range(17, 24)),
    ("Cost",                  range(24, 25)),
    ("Releases",              range(25, 28)),
    ("GitHub Bootstrap",      range(28, 30)),
    ("Kubeconfig",            range(30, 32)),
    ("GitHub Webhook",        range(32, 38)),
    ("Swagger / OpenAPI",     range(38, 42)),
]

def camel_to_words(s: str) -> str:
    """Convert CamelCase to space-separated words."""
    s = re.sub(r"([A-Z][a-z]+)", r" \1", s).strip()
    return s

def parse_test_name(name: str):
    """Return (id_str, description) from TestT001_RegisterNewUser."""
    m = re.match(r"TestT(\d+)_(.+)", name)
    if m:
        num = int(m.group(1))
        desc = camel_to_words(m.group(2))
        return f"T-{m.group(1).zfill(3)}", num, desc
    return None, None, name

def parse(input_path: str):
    results = {}   # test_name -> {id, num, desc, action, elapsed, output}
    output_lines = {}  # test_name -> [lines]

    with open(input_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            action = obj.get("Action", "")
            test = obj.get("Test", "")
            if not test:
                continue
            # Skip subtest entries
            if "/" in test:
                continue
            id_str, num, desc = parse_test_name(test)
            if id_str is None:
                continue
            if test not in results:
                results[test] = {
                    "id": id_str,
                    "num": num,
                    "desc": desc,
                    "action": "run",
                    "elapsed": 0.0,
                }
                output_lines[test] = []
            if action in ("pass", "fail", "skip"):
                results[test]["action"] = action
                results[test]["elapsed"] = obj.get("Elapsed", 0.0)
            if action == "output":
                output_lines[test].append(obj.get("Output", ""))

    return results, output_lines

def build_report(results, output_lines) -> str:
    total = len(results)
    passed = sum(1 for r in results.values() if r["action"] == "pass")
    failed = total - passed

    lines = []
    today = date.today().strftime("%d.%m.%Y")
    lines.append(f"# Отчёт о тестировании Deploy Service")
    lines.append(f"**Дата:** {today}  ")
    lines.append("**Тестировщик:** автоматический запуск (скрипт)  ")
    lines.append(f"**Метод:** `go test -v ./tests/api/...` (httptest.NewServer, in-memory store, mock k8s/github)")
    lines.append("")
    lines.append("---")
    lines.append("")
    lines.append("## Итог")
    lines.append("")
    lines.append("| Всего | Пройдено | Провалено |")
    lines.append(f"|-------|-----------|-----------|")
    lines.append(f"| {total} | {passed} | {failed} |")
    lines.append("")

    for cat_name, id_range in CATEGORIES:
        cat_tests = sorted(
            [r for r in results.values() if r["num"] in id_range],
            key=lambda x: x["num"],
        )
        if not cat_tests:
            continue
        lines.append(f"## {cat_name}")
        lines.append("")
        lines.append("| ID | Тест | Время (с) | Статус |")
        lines.append("|-----|------|-----------|--------|")
        for r in cat_tests:
            icon = "ПРОЙДЕН" if r["action"] == "pass" else "ПРОВАЛЕН"
            elapsed = f"{r['elapsed']:.3f}"
            lines.append(f"| {r['id']} | {r['desc']} | {elapsed} | {icon} |")
        lines.append("")

        # Print failure details
        for r in cat_tests:
            if r["action"] != "pass":
                test_key = next(
                    k for k, v in results.items() if v["id"] == r["id"]
                )
                out = "".join(output_lines.get(test_key, []))
                if out.strip():
                    lines.append(f"<details><summary>{r['id']} failure output</summary>")
                    lines.append("")
                    lines.append("```")
                    lines.append(out.strip())
                    lines.append("```")
                    lines.append("</details>")
                    lines.append("")

    return "\n".join(lines) + "\n"

def main():
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <input.json> <output.md>", file=sys.stderr)
        sys.exit(1)

    input_path = sys.argv[1]
    output_path = sys.argv[2]

    results, output_lines = parse(input_path)
    report = build_report(results, output_lines)

    Path(output_path).parent.mkdir(parents=True, exist_ok=True)
    Path(output_path).write_text(report)

    total = len(results)
    passed = sum(1 for r in results.values() if r["action"] == "pass")
    print(f"Results: {passed}/{total} passed")

if __name__ == "__main__":
    main()
