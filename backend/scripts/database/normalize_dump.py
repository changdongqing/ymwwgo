#!/usr/bin/env python3
"""Normalize pg_dump --schema-only output before drift diffing.

Two order-only differences are expected when a column is added:

1. PostgreSQL appends ADD COLUMN at the end of a table, while Ent places edge
   fields (e.g. parent_id, declared by mixin.Tree) after regular fields. Column
   order is not part of the logical schema contract.
2. pg_dump emits COMMENT statements in object order (attnum for columns), so a
   comment moves together with its column.

The normalizer sorts column lines inside each CREATE TABLE block and sorts
COMMENT blocks by their statement. It only ignores order, not missing/extra
columns, types, defaults or comment text.
"""
import sys


def normalize_table_columns(lines: list[str]) -> list[str]:
    out: list[str] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        out.append(line)
        if line.startswith("CREATE TABLE ") and line.rstrip().endswith("("):
            body: list[str] = []
            i += 1
            while i < len(lines) and lines[i].strip() != ");":
                body.append(lines[i])
                i += 1

            columns = [
                row
                for row in body
                if row.startswith("    ") and not row.lstrip().startswith("CONSTRAINT ")
            ]
            others = [
                row
                for row in body
                if not (row.startswith("    ") and not row.lstrip().startswith("CONSTRAINT "))
            ]
            # 逗号跟着“原表尾”走，排序前先摘掉，排序后按新位置补回，
            # 否则 diff 会把逗号所在行的移动当成结构差异。
            ordered = sorted(row.rstrip().rstrip(",") for row in columns)
            if ordered:
                has_constraints = any(row.lstrip().startswith("CONSTRAINT ") for row in others)
                ordered = [row + "," for row in ordered[:-1]] + [ordered[-1]]
                if has_constraints:
                    ordered = [row + "," for row in ordered]
            out.extend(ordered)
            out.extend(others)

            if i < len(lines):
                out.append(lines[i])
                i += 1
            continue
        i += 1
    return out


def is_comment_header(line: str) -> bool:
    return line.startswith("-- Name: ") and "; Type: COMMENT;" in line


def normalize_comments(lines: list[str]) -> list[str]:
    out: list[str] = []
    blocks: list[tuple[str, list[str]]] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        if line == "--" and i + 1 < len(lines) and is_comment_header(lines[i + 1]):
            start = i
            i += 2
            if i < len(lines) and lines[i] == "--":
                i += 1
            while i < len(lines) and lines[i] == "":
                i += 1
            if i < len(lines) and lines[i].startswith("COMMENT ON "):
                stmt = lines[i]
                i += 1
                while i < len(lines) and lines[i] == "":
                    i += 1
                blocks.append((stmt, lines[start:i]))
                continue
            out.extend(lines[start:i])
            continue
        out.append(line)
        i += 1

    blocks.sort(key=lambda block: block[0])
    for _, block in blocks:
        out.extend(block)
    return out


if __name__ == "__main__":
    text = normalize_table_columns(sys.stdin.read().splitlines())
    text = normalize_comments(text)
    sys.stdout.write("\n".join(text) + "\n")
