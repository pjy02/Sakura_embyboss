#!/usr/bin/env python3
"""Create a read-only, value-free profile of a live Sakura v2 MySQL schema."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path


def required_env(primary: str, fallback: str | None = None) -> str:
    value = os.getenv(primary, "")
    if not value and fallback:
        value = os.getenv(fallback, "")
    if not value:
        raise RuntimeError(f"missing environment variable: {primary}")
    return value


def quote_identifier(value: str) -> str:
    return "`" + value.replace("`", "``") + "`"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, help="write JSON here; stdout is used when omitted")
    parser.add_argument(
        "--exact-counts",
        action="store_true",
        help="run SELECT COUNT(*) for every table; this can be slow on large production tables",
    )
    args = parser.parse_args()

    try:
        import pymysql
    except ImportError:
        print("PyMySQL is required; install the project requirements first", file=sys.stderr)
        return 2

    try:
        database = required_env("SAKURA_DB_NAME", "MYSQL_DATABASE")
        connection = pymysql.connect(
            host=os.getenv("SAKURA_DB_HOST", "127.0.0.1"),
            port=int(os.getenv("SAKURA_DB_PORT", "3306")),
            user=required_env("SAKURA_DB_USER", "MYSQL_USER"),
            password=required_env("SAKURA_DB_PASSWORD", "MYSQL_PASSWORD"),
            database=database,
            charset="utf8mb4",
            cursorclass=pymysql.cursors.DictCursor,
            read_timeout=30,
            write_timeout=30,
            autocommit=True,
        )
    except Exception as error:
        print(f"database connection failed: {error}", file=sys.stderr)
        return 2

    try:
        with connection.cursor() as cursor:
            cursor.execute("SELECT VERSION() AS version")
            version = cursor.fetchone()["version"]
            cursor.execute(
                """
                SELECT TABLE_NAME, ENGINE, TABLE_ROWS, DATA_LENGTH, INDEX_LENGTH,
                       TABLE_COLLATION
                FROM information_schema.TABLES
                WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE'
                ORDER BY TABLE_NAME
                """,
                (database,),
            )
            table_rows = cursor.fetchall()
            cursor.execute(
                """
                SELECT TABLE_NAME, ORDINAL_POSITION, COLUMN_NAME, COLUMN_TYPE,
                       IS_NULLABLE, COLUMN_KEY, EXTRA
                FROM information_schema.COLUMNS
                WHERE TABLE_SCHEMA = %s
                ORDER BY TABLE_NAME, ORDINAL_POSITION
                """,
                (database,),
            )
            columns = cursor.fetchall()
            cursor.execute(
                """
                SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
                FROM information_schema.STATISTICS
                WHERE TABLE_SCHEMA = %s
                ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX
                """,
                (database,),
            )
            indexes = cursor.fetchall()

            exact_counts: dict[str, int] = {}
            if args.exact_counts:
                for table in table_rows:
                    name = table["TABLE_NAME"]
                    cursor.execute(f"SELECT COUNT(*) AS row_count FROM {quote_identifier(name)}")
                    exact_counts[name] = int(cursor.fetchone()["row_count"])
    finally:
        connection.close()

    by_table: dict[str, dict] = {}
    for row in table_rows:
        name = row.pop("TABLE_NAME")
        by_table[name] = {
            "engine": row["ENGINE"],
            "estimated_rows": int(row["TABLE_ROWS"] or 0),
            "exact_rows": exact_counts.get(name),
            "data_bytes": int(row["DATA_LENGTH"] or 0),
            "index_bytes": int(row["INDEX_LENGTH"] or 0),
            "collation": row["TABLE_COLLATION"],
            "columns": [],
            "indexes": [],
        }
    for row in columns:
        name = row.pop("TABLE_NAME")
        if name in by_table:
            by_table[name]["columns"].append({key.lower(): value for key, value in row.items()})
    for row in indexes:
        name = row.pop("TABLE_NAME")
        if name in by_table:
            by_table[name]["indexes"].append({key.lower(): value for key, value in row.items()})

    fingerprint_source = json.dumps(by_table, ensure_ascii=False, sort_keys=True, default=str).encode("utf-8")
    result = {
        "schema_version": 1,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "database_product": "MySQL",
        "database_version": version,
        "database_name_hash": hashlib.sha256(database.encode("utf-8")).hexdigest(),
        "contains_row_values": False,
        "exact_counts_requested": args.exact_counts,
        "schema_fingerprint": hashlib.sha256(fingerprint_source).hexdigest(),
        "summary": {
            "tables": len(by_table),
            "columns": sum(len(item["columns"]) for item in by_table.values()),
            "indexes": sum(len(item["indexes"]) for item in by_table.values()),
        },
        "tables": by_table,
    }
    content = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True, default=str) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(content, encoding="utf-8")
        print(f"wrote database profile: {args.output}")
    else:
        sys.stdout.write(content)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
