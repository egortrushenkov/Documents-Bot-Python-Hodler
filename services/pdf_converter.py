"""
Конвертация DOCX → PDF через LibreOffice headless.

Особенности многопользовательской работы:
- семафор ограничивает число одновременных конвертаций (LibreOffice тяжёлый);
- каждая конвертация получает собственный профиль (-env:UserInstallation) —
  два LibreOffice с общим профилем блокируют друг друга и «подвешивают» бота;
- таймаут, чтобы зависший процесс не держал очередь вечно.
"""
import asyncio
import os
import shutil
import tempfile
import uuid
from config import LIBREOFFICE_PATH, OUTPUT_DIR, PDF_CONCURRENCY

_semaphore = asyncio.Semaphore(max(1, PDF_CONCURRENCY))
CONVERT_TIMEOUT = 120  # секунд


async def convert_to_pdf(docx_path: str) -> str:
    """
    Асинхронно конвертирует DOCX в PDF.
    Возвращает путь к PDF.
    """
    out_dir = os.path.dirname(docx_path) or OUTPUT_DIR
    profile_dir = os.path.join(tempfile.gettempdir(), f"lo_profile_{uuid.uuid4().hex}")
    cmd = [
        LIBREOFFICE_PATH, "--headless", "--norestore",
        f"-env:UserInstallation=file://{profile_dir}",
        "--convert-to", "pdf",
        "--outdir", out_dir,
        docx_path,
    ]
    async with _semaphore:
        proc = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        try:
            stdout, stderr = await asyncio.wait_for(
                proc.communicate(), timeout=CONVERT_TIMEOUT
            )
        except asyncio.TimeoutError:
            proc.kill()
            await proc.wait()
            shutil.rmtree(profile_dir, ignore_errors=True)
            raise RuntimeError(f"LibreOffice не ответил за {CONVERT_TIMEOUT} с — конвертация прервана")

    shutil.rmtree(profile_dir, ignore_errors=True)

    if proc.returncode != 0:
        raise RuntimeError(
            f"LibreOffice вернул код {proc.returncode}.\n"
            f"stderr: {stderr.decode()}"
        )

    pdf_path = docx_path.replace(".docx", ".pdf")
    if not os.path.exists(pdf_path):
        raise FileNotFoundError(f"PDF не создан: {pdf_path}")
    return pdf_path
