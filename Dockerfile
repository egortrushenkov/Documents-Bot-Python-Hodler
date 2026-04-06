FROM python:3.11-slim

# LibreOffice для конвертации в PDF
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice \
    libreoffice-writer \
    fonts-dejavu \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY bot/ ./bot/

# data/ монтируется как volume (шаблон + база клиентов + output)
VOLUME ["/app/data"]

CMD ["python", "-m", "bot.main"]
