"""Мелкие общие помощники."""
import html


def esc(value) -> str:
    """Экранирует пользовательский текст для parse_mode=HTML."""
    return html.escape(str(value)) if value is not None else ""


def safe_name_part(name: str, limit: int = 20, default: str = "client") -> str:
    """Часть имени файла из названия компании: только буквы/цифры/_-."""
    cleaned = "".join(
        ch if (ch.isalnum() or ch in "_-.") else "_"
        for ch in (name or default)
    )
    while "__" in cleaned:
        cleaned = cleaned.replace("__", "_")
    return cleaned.strip("_")[:limit] or default
