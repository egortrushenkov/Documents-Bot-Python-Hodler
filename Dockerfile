FROM python:3.11-slim

# LibreOffice для PDF-конвертации
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreoffice \
    libreoffice-writer \
    fonts-liberation \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# Подготовка шаблонов при первом запуске
RUN mkdir -p data/output templates

CMD ["sh", "-c", "python prepare_templates.py && python bot.py"]
