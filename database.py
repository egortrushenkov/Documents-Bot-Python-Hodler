import aiosqlite
import os
import json
from typing import Optional, List, Dict, Any
from config import DB_PATH


def _connect():
    # timeout = busy_timeout: несколько пользователей могут писать одновременно
    return aiosqlite.connect(DB_PATH, timeout=30)


async def _ensure_column(db, table: str, column: str, ddl: str):
    async with db.execute(f"PRAGMA table_info({table})") as cur:
        cols = [r[1] for r in await cur.fetchall()]
    if column not in cols:
        await db.execute(f"ALTER TABLE {table} ADD COLUMN {ddl}")


async def init_db():
    os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
    async with _connect() as db:
        # WAL: параллельные чтения не блокируются записью
        await db.execute("PRAGMA journal_mode=WAL")
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

        CREATE TABLE IF NOT EXISTS deals (
            id              INTEGER PRIMARY KEY AUTOINCREMENT,
            created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            act_type        TEXT NOT NULL,
            deal_number     TEXT DEFAULT '',
            deal_date       TEXT DEFAULT '',
            deal_date_iso   TEXT DEFAULT '',
            execution_date  TEXT DEFAULT '',
            company_id      INTEGER,
            client_name     TEXT DEFAULT '',
            client_inn      TEXT DEFAULT '',
            client_resident TEXT DEFAULT '',
            va_type         TEXT DEFAULT '',
            network         TEXT DEFAULT '',
            va_amount       REAL DEFAULT 0,
            fiat_amount     REAL DEFAULT 0,
            fiat_currency   TEXT DEFAULT 'RUB',
            exchange_rate   TEXT DEFAULT '',
            kvvo            TEXT DEFAULT '',
            client_wallet   TEXT DEFAULT '',
            operator_wallet TEXT DEFAULT '',
            commission_fiat TEXT DEFAULT '',
            commission_va   TEXT DEFAULT '',
            transactions    TEXT DEFAULT '[]',
            docx_file_id    TEXT DEFAULT '',
            pdf_file_id     TEXT DEFAULT '',
            docx_name       TEXT DEFAULT '',
            pdf_name        TEXT DEFAULT ''
        );
        CREATE INDEX IF NOT EXISTS idx_deals_month   ON deals (deal_date_iso);
        CREATE INDEX IF NOT EXISTS idx_deals_company ON deals (company_id);

        CREATE TABLE IF NOT EXISTS signed_docs (
            id          INTEGER PRIMARY KEY AUTOINCREMENT,
            company_id  INTEGER,
            deal_id     INTEGER,
            file_id     TEXT NOT NULL,
            file_name   TEXT DEFAULT '',
            uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        CREATE INDEX IF NOT EXISTS idx_signed_company ON signed_docs (company_id);

        CREATE TABLE IF NOT EXISTS company_counters (
            company_id  INTEGER,
            kind        TEXT,               -- act | invoice
            last_number INTEGER,
            PRIMARY KEY (company_id, kind)
        );

        CREATE TABLE IF NOT EXISTS custom_fields (
            key        TEXT PRIMARY KEY,    -- латиницей, доступен как {{CL_KEY}}
            label      TEXT NOT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );

        CREATE TABLE IF NOT EXISTS company_custom (
            company_id INTEGER,
            key        TEXT,
            value      TEXT DEFAULT '',
            PRIMARY KEY (company_id, key)
        );
        """)

        # Миграция существующей базы
        await _ensure_column(db, "companies", "resident", "resident TEXT DEFAULT 'Резидент'")

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
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(
            "SELECT * FROM companies ORDER BY short_name"
        ) as cur:
            rows = await cur.fetchall()
    return [dict(r) for r in rows]


async def get_company(company_id: int) -> Optional[Dict[str, Any]]:
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(
            "SELECT * FROM companies WHERE id=?", (company_id,)
        ) as cur:
            row = await cur.fetchone()
    return dict(row) if row else None


async def add_company(data: Dict[str, Any]) -> int:
    async with _connect() as db:
        cur = await db.execute(
            """INSERT INTO companies
            (full_name,short_name,inn,kpp,reg_number,address,kio,inn_rf,kpp_rf,
             bank_name,bank_account,bank_bik,wallet,resident)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
            (
                data.get("full_name",""), data.get("short_name",""),
                data.get("inn",""), data.get("kpp",""),
                data.get("reg_number",""), data.get("address",""),
                data.get("kio","-"), data.get("inn_rf","-"),
                data.get("kpp_rf","-"), data.get("bank_name",""),
                data.get("bank_account",""), data.get("bank_bik",""),
                data.get("wallet",""), data.get("resident","Резидент"),
            )
        )
        await db.commit()
        return cur.lastrowid


async def update_company(company_id: int, data: Dict[str, Any]):
    fields = ", ".join(f"{k}=?" for k in data)
    values = list(data.values()) + [company_id]
    async with _connect() as db:
        await db.execute(
            f"UPDATE companies SET {fields} WHERE id=?", values
        )
        await db.commit()


async def delete_company(company_id: int):
    async with _connect() as db:
        await db.execute("DELETE FROM companies WHERE id=?", (company_id,))
        await db.execute("DELETE FROM company_custom WHERE company_id=?", (company_id,))
        await db.execute("DELETE FROM company_counters WHERE company_id=?", (company_id,))
        await db.commit()


# ─── Custom fields (свои переменные для компаний) ────────────────────────────

async def get_custom_fields() -> List[Dict[str, str]]:
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(
            "SELECT key, label FROM custom_fields ORDER BY created_at"
        ) as cur:
            rows = await cur.fetchall()
    return [dict(r) for r in rows]


async def add_custom_field(key: str, label: str):
    async with _connect() as db:
        await db.execute(
            "INSERT OR REPLACE INTO custom_fields (key, label) VALUES (?,?)",
            (key, label)
        )
        await db.commit()


async def delete_custom_field(key: str):
    async with _connect() as db:
        await db.execute("DELETE FROM custom_fields WHERE key=?", (key,))
        await db.execute("DELETE FROM company_custom WHERE key=?", (key,))
        await db.commit()


async def get_company_custom(company_id: int) -> Dict[str, str]:
    async with _connect() as db:
        async with db.execute(
            "SELECT key, value FROM company_custom WHERE company_id=?", (company_id,)
        ) as cur:
            rows = await cur.fetchall()
    return {r[0]: r[1] for r in rows}


async def set_company_custom(company_id: int, key: str, value: str):
    async with _connect() as db:
        await db.execute(
            "INSERT OR REPLACE INTO company_custom (company_id, key, value) VALUES (?,?,?)",
            (company_id, key, value)
        )
        await db.commit()


# ─── Operator settings ────────────────────────────────────────────────────────

async def get_all_settings() -> Dict[str, str]:
    async with _connect() as db:
        async with db.execute("SELECT key, value FROM operator_settings") as cur:
            rows = await cur.fetchall()
    return {r[0]: r[1] for r in rows}


async def set_setting(key: str, value: str):
    async with _connect() as db:
        await db.execute(
            "INSERT OR REPLACE INTO operator_settings (key, value) VALUES (?,?)",
            (key, value)
        )
        await db.commit()


# ─── Numbering (общий счётчик + по клиентам) ─────────────────────────────────

async def peek_next_number() -> int:
    """Следующий общий номер без инкремента"""
    async with _connect() as db:
        async with db.execute(
            "SELECT last_number FROM act_counter WHERE id=1"
        ) as cur:
            row = await cur.fetchone()
    return (row[0] if row else 499) + 1


async def peek_company_number(company_id: int, kind: str) -> Optional[int]:
    """Следующий номер для клиента (kind: act|invoice) или None, если истории нет"""
    async with _connect() as db:
        async with db.execute(
            "SELECT last_number FROM company_counters WHERE company_id=? AND kind=?",
            (company_id, kind)
        ) as cur:
            row = await cur.fetchone()
    return (row[0] + 1) if row else None


async def record_number_used(company_id: int, kind: str, number) -> None:
    """
    Фиксирует использованный номер: двигает счётчик клиента и общий счётчик
    вперёд (только вперёд — назад не откатывает). Нечисловые номера игнорируются.
    """
    try:
        n = int(str(number).strip().lstrip("№").strip())
    except (ValueError, TypeError):
        return
    async with _connect() as db:
        await db.execute(
            """INSERT INTO company_counters (company_id, kind, last_number)
               VALUES (?,?,?)
               ON CONFLICT(company_id, kind)
               DO UPDATE SET last_number = MAX(last_number, excluded.last_number)""",
            (company_id, kind, n)
        )
        await db.execute(
            "UPDATE act_counter SET last_number = MAX(last_number, ?) WHERE id=1", (n,)
        )
        await db.commit()


# ─── Custom KVVO codes ────────────────────────────────────────────────────────

async def get_kvvo_codes() -> List[str]:
    async with _connect() as db:
        async with db.execute("SELECT code FROM kvvo_codes ORDER BY code") as cur:
            rows = await cur.fetchall()
    return [r[0] for r in rows]


async def add_kvvo(code: str):
    async with _connect() as db:
        await db.execute(
            "INSERT OR IGNORE INTO kvvo_codes (code) VALUES (?)", (code,)
        )
        await db.commit()


async def delete_kvvo(code: str):
    async with _connect() as db:
        await db.execute("DELETE FROM kvvo_codes WHERE code=?", (code,))
        await db.commit()


# ─── Deals (журнал сделок) ────────────────────────────────────────────────────

_DEAL_FIELDS = [
    "act_type", "deal_number", "deal_date", "deal_date_iso", "execution_date",
    "company_id", "client_name", "client_inn", "client_resident",
    "va_type", "network", "va_amount", "fiat_amount", "fiat_currency",
    "exchange_rate", "kvvo", "client_wallet", "operator_wallet",
    "commission_fiat", "commission_va", "transactions",
    "docx_file_id", "pdf_file_id", "docx_name", "pdf_name",
]


async def add_deal(data: Dict[str, Any]) -> int:
    cols = ",".join(_DEAL_FIELDS)
    marks = ",".join("?" * len(_DEAL_FIELDS))
    values = [data.get(f, "") for f in _DEAL_FIELDS]
    async with _connect() as db:
        cur = await db.execute(
            f"INSERT INTO deals ({cols}) VALUES ({marks})", values
        )
        await db.commit()
        return cur.lastrowid


async def get_deal(deal_id: int) -> Optional[Dict[str, Any]]:
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute("SELECT * FROM deals WHERE id=?", (deal_id,)) as cur:
            row = await cur.fetchone()
    return dict(row) if row else None


async def get_recent_deals(limit: int = 10, company_id: Optional[int] = None) -> List[Dict[str, Any]]:
    q = "SELECT * FROM deals"
    args: list = []
    if company_id:
        q += " WHERE company_id=?"
        args.append(company_id)
    q += " ORDER BY id DESC LIMIT ?"
    args.append(limit)
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(q, args) as cur:
            rows = await cur.fetchall()
    return [dict(r) for r in rows]


async def get_deals_for_month(month: str) -> List[Dict[str, Any]]:
    """month = 'YYYY-MM'. Возвращает сделки месяца по дате исполнения (заявки)."""
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(
            "SELECT * FROM deals WHERE substr(deal_date_iso,1,7)=? ORDER BY deal_date_iso, id",
            (month,)
        ) as cur:
            rows = await cur.fetchall()
    return [dict(r) for r in rows]


async def get_deal_months() -> List[str]:
    """Список месяцев 'YYYY-MM', по которым есть сделки (новые сверху)."""
    async with _connect() as db:
        async with db.execute(
            """SELECT DISTINCT substr(deal_date_iso,1,7) AS m FROM deals
               WHERE deal_date_iso != '' ORDER BY m DESC LIMIT 12"""
        ) as cur:
            rows = await cur.fetchall()
    return [r[0] for r in rows]


async def delete_deal(deal_id: int):
    async with _connect() as db:
        await db.execute("DELETE FROM deals WHERE id=?", (deal_id,))
        await db.execute("DELETE FROM signed_docs WHERE deal_id=?", (deal_id,))
        await db.commit()


# ─── Signed docs (подписанные акты: хранится только file_id Telegram) ─────────

async def add_signed_doc(company_id: Optional[int], deal_id: Optional[int],
                         file_id: str, file_name: str) -> int:
    async with _connect() as db:
        cur = await db.execute(
            "INSERT INTO signed_docs (company_id, deal_id, file_id, file_name) VALUES (?,?,?,?)",
            (company_id, deal_id, file_id, file_name)
        )
        await db.commit()
        return cur.lastrowid


async def get_signed_docs(company_id: Optional[int] = None,
                          deal_id: Optional[int] = None) -> List[Dict[str, Any]]:
    q, args = "SELECT * FROM signed_docs", []
    if deal_id is not None:
        q += " WHERE deal_id=?"
        args.append(deal_id)
    elif company_id is not None:
        q += " WHERE company_id=?"
        args.append(company_id)
    q += " ORDER BY uploaded_at"
    async with _connect() as db:
        db.row_factory = aiosqlite.Row
        async with db.execute(q, args) as cur:
            rows = await cur.fetchall()
    return [dict(r) for r in rows]


async def count_signed_docs(deal_id: int) -> int:
    async with _connect() as db:
        async with db.execute(
            "SELECT COUNT(*) FROM signed_docs WHERE deal_id=?", (deal_id,)
        ) as cur:
            row = await cur.fetchone()
    return row[0] if row else 0
