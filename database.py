import aiosqlite
import os
from typing import Optional, List, Dict, Any
from config import DB_PATH


async def init_db():
    os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
    async with aiosqlite.connect(DB_PATH) as db:
        await db.executescript("""
        CREATE TABLE IF NOT EXISTS companies (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            full_name   TEXT NOT NULL,
            short_name  TEXT NOT NULL,
            inn         TEXT DEFAULT '',
            kpp         TEXT DEFAULT '',
            reg_number  TEXT DEFAULT '',
            address     TEXT DEFAULT '',
            kio         TEXT DEFAULT '-',
            inn_rf      TEXT DEFAULT '-',
            kpp_rf      TEXT DEFAULT '-',
            bank_name   TEXT DEFAULT '',
            bank_account TEXT DEFAULT '',
            bank_bik    TEXT DEFAULT '',
            wallet      TEXT DEFAULT '',
            created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS operator_settings (
            key   TEXT PRIMARY KEY,
            value TEXT
        );

        CREATE TABLE IF NOT EXISTS act_counter (
            id          INTEGER PRIMARY KEY,
            last_number INTEGER DEFAULT 499
        );

        CREATE TABLE IF NOT EXISTS kvvo_codes (
            code       TEXT PRIMARY KEY,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        """)

        # Default operator data
        defaults = {
            "full_name":     "Общество с ограниченной ответственностью «СТЕЙБЛЕКС»",
            "short_name":    "ОсОО «СТЕЙБЛЕКС»",
            "inn":           "9909730748",
            "kpp":           "770387001",
            "address":       "Кыргызская Республика, г. Бишкек, ул. Московская, д. 197",
            "legal_address": "720009, г. Бишкек, ул. Московская, д. 197",
            "license":       "VA №0171",
            "kio":           "73074",
            "inn_rf":        "9909730748",
            "kpp_rf":        "770387001",
            "bank_name":     "КБ \"Долинск\" (АО)",
            "bank_account":  "40807810500014264602",
            "bank_bik":      "046401727",
            "wallet":        "TXFEYN4C5BnesaxUXJiXJHGS7K12QutZ3r",
            "director_name": "Зенков И.В.",
            "director_title":"Директор",
        }
        for key, value in defaults.items():
            await db.execute(
                "INSERT OR IGNORE INTO operator_settings (key, value) VALUES (?, ?)",
                (key, value)
            )
        await db.execute("INSERT OR IGNORE INTO act_counter VALUES (1, 499)")
        await db.commit()


# ─── Companies ────────────────────────────────────────────────────────────────

async def get_all_companies() -> List[Dict[str, Any]]:
    async with aiosqlite.connect(DB_PATH) as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(
            "SELECT * FROM companies ORDER BY short_name"
        ) as cur:
            rows = await cur.fetchall()
    return [dict(r) for r in rows]


async def get_company(company_id: int) -> Optional[Dict[str, Any]]:
    async with aiosqlite.connect(DB_PATH) as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(
            "SELECT * FROM companies WHERE id=?", (company_id,)
        ) as cur:
            row = await cur.fetchone()
    return dict(row) if row else None


async def add_company(data: Dict[str, Any]) -> int:
    async with aiosqlite.connect(DB_PATH) as db:
        cur = await db.execute(
            """INSERT INTO companies
            (full_name,short_name,inn,kpp,reg_number,address,kio,inn_rf,kpp_rf,
             bank_name,bank_account,bank_bik,wallet)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)""",
            (
                data.get("full_name",""), data.get("short_name",""),
                data.get("inn",""), data.get("kpp",""),
                data.get("reg_number",""), data.get("address",""),
                data.get("kio","-"), data.get("inn_rf","-"),
                data.get("kpp_rf","-"), data.get("bank_name",""),
                data.get("bank_account",""), data.get("bank_bik",""),
                data.get("wallet",""),
            )
        )
        await db.commit()
        return cur.lastrowid


async def update_company(company_id: int, data: Dict[str, Any]):
    fields = ", ".join(f"{k}=?" for k in data)
    values = list(data.values()) + [company_id]
    async with aiosqlite.connect(DB_PATH) as db:
        await db.execute(
            f"UPDATE companies SET {fields} WHERE id=?", values
        )
        await db.commit()


async def delete_company(company_id: int):
    async with aiosqlite.connect(DB_PATH) as db:
        await db.execute("DELETE FROM companies WHERE id=?", (company_id,))
        await db.commit()


# ─── Operator settings ────────────────────────────────────────────────────────

async def get_all_settings() -> Dict[str, str]:
    async with aiosqlite.connect(DB_PATH) as db:
        async with db.execute("SELECT key, value FROM operator_settings") as cur:
            rows = await cur.fetchall()
    return {r[0]: r[1] for r in rows}


async def set_setting(key: str, value: str):
    async with aiosqlite.connect(DB_PATH) as db:
        await db.execute(
            "INSERT OR REPLACE INTO operator_settings (key, value) VALUES (?,?)",
            (key, value)
        )
        await db.commit()


# ─── Act counter ──────────────────────────────────────────────────────────────

async def next_act_number() -> int:
    async with aiosqlite.connect(DB_PATH) as db:
        async with db.execute(
            "SELECT last_number FROM act_counter WHERE id=1"
        ) as cur:
            row = await cur.fetchone()
        n = (row[0] if row else 499) + 1
        await db.execute("UPDATE act_counter SET last_number=? WHERE id=1", (n,))
        await db.commit()
    return n


async def peek_next_number() -> int:
    """Next number without incrementing"""
    async with aiosqlite.connect(DB_PATH) as db:
        async with db.execute(
            "SELECT last_number FROM act_counter WHERE id=1"
        ) as cur:
            row = await cur.fetchone()
    return (row[0] if row else 499) + 1


# ─── Custom KVVO codes ────────────────────────────────────────────────────────

async def get_kvvo_codes() -> List[str]:
    async with aiosqlite.connect(DB_PATH) as db:
        async with db.execute("SELECT code FROM kvvo_codes ORDER BY code") as cur:
            rows = await cur.fetchall()
    return [r[0] for r in rows]


async def add_kvvo(code: str):
    async with aiosqlite.connect(DB_PATH) as db:
        await db.execute(
            "INSERT OR IGNORE INTO kvvo_codes (code) VALUES (?)", (code,)
        )
        await db.commit()


async def delete_kvvo(code: str):
    async with aiosqlite.connect(DB_PATH) as db:
        await db.execute("DELETE FROM kvvo_codes WHERE code=?", (code,))
        await db.commit()
