from __future__ import annotations

from typing import Any


def _active_part_rows(db: Any, table_name: str) -> int:
    row = db.execute(
        "SELECT COALESCE(sum(rows), 0) AS c "
        "FROM system.parts "
        "WHERE active = 1 AND database = currentDatabase() AND table = ?",
        [table_name],
    ).fetchone()
    if not row:
        return 0
    return int(row["c"] or 0)


def _get_db_stats(db: Any, *, log_debug: Any) -> dict[str, Any]:
    stats: dict[str, Any] = {
        "compressed_bytes": None,
        "uncompressed_bytes": None,
        "compression_ratio": None,
        "total_rows": None,
        "active_queries": None,
        "tables": [],
    }
    try:
        row = db.execute(
            "SELECT "
            "  sum(data_compressed_bytes)   AS comp, "
            "  sum(data_uncompressed_bytes) AS uncomp, "
            "  sum(rows)                    AS rws "
            "FROM system.parts "
            "WHERE active = 1 AND database = currentDatabase()"
        ).fetchone()
        if row:
            compressed = int(row["comp"] or 0)
            uncompressed = int(row["uncomp"] or 0)
            stats["compressed_bytes"] = compressed
            stats["uncompressed_bytes"] = uncompressed
            stats["total_rows"] = int(row["rws"] or 0)
            if compressed > 0:
                stats["compression_ratio"] = round(uncompressed / compressed, 2)
    except Exception:
        log_debug("db_stats: system.parts query failed", exc_info=True)

    try:
        rows = db.execute(
            "SELECT table, "
            "  sum(data_compressed_bytes)   AS comp, "
            "  sum(data_uncompressed_bytes) AS uncomp, "
            "  sum(rows)                    AS rws "
            "FROM system.parts "
            "WHERE active = 1 AND database = currentDatabase() "
            "GROUP BY table "
            "ORDER BY comp DESC "
            "LIMIT 10"
        ).fetchall()
        table_stats = []
        for row in rows:
            compressed = int(row["comp"] or 0)
            uncompressed = int(row["uncomp"] or 0)
            table_stats.append(
                {
                    "table": row["table"],
                    "compressed_bytes": compressed,
                    "uncompressed_bytes": uncompressed,
                    "rows": int(row["rws"] or 0),
                    "compression_ratio": round(uncompressed / compressed, 2) if compressed > 0 else None,
                }
            )
        stats["tables"] = table_stats
    except Exception:
        log_debug("db_stats: per-table system.parts query failed", exc_info=True)

    try:
        row = db.execute("SELECT COUNT(*) AS cnt FROM system.processes").fetchone()
        if row:
            stats["active_queries"] = int(row["cnt"] or 0)
    except Exception:
        log_debug("db_stats: system.processes query failed", exc_info=True)

    return stats


def _fmt_bytes(n: int | None) -> str:
    if n is None:
        return "—"
    if n >= 1024**3:
        return f"{n / 1024 ** 3:.1f} GB"
    if n >= 1024**2:
        return f"{n / 1024 ** 2:.1f} MB"
    if n >= 1024:
        return f"{n / 1024:.1f} KB"
    return f"{n} B"
