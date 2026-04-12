"""
Конвертация DOCX → PDF через LibreOffice headless.
"""
import asyncio
import os
import subprocess
from config import LIBREOFFICE_PATH, OUTPUT_DIR


async def convert_to_pdf(docx_path: str) -> str:
    """
    Асинхронно конвертирует DOCX в PDF.
    Возвращает путь к PDF.
    """
    out_dir = os.path.dirname(docx_path) or OUTPUT_DIR
    cmd = [
        LIBREOFFICE_PATH, "--headless", "--norestore",
        "--convert-to", "pdf",
        "--outdir", out_dir,
        docx_path,
    ]
    proc = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, stderr = await proc.communicate()

    if proc.returncode != 0:
        raise RuntimeError(
            f"LibreOffice вернул код {proc.returncode}.\n"
            f"stderr: {stderr.decode()}"
        )

    pdf_path = docx_path.replace(".docx", ".pdf")
    if not os.path.exists(pdf_path):
        raise FileNotFoundError(f"PDF не создан: {pdf_path}")
    return pdf_path
