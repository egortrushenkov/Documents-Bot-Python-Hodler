FROM python:3.11-slim

# LibreOffice + Microsoft-совместимые шрифты (Calibri, Verdana и др.)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice \
    libreoffice-writer \
    fonts-crosextra-carlito \
    fonts-crosextra-caladea \
    ttf-mscorefonts-installer \
    fontconfig \
    && fc-cache -fv \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY bot/ ./bot/

VOLUME ["/app/data"]

CMD ["python", "-m", "bot.main"]
