FROM python:3.11-slim

# 1) Включаем contrib-репозиторий (нужен для ttf-mscorefonts-installer)
RUN sed -i 's/^Components: main$/Components: main contrib/' /etc/apt/sources.list.d/debian.sources

# 2) Автоматически принимаем EULA перед установкой + ставим всё
RUN apt-get update && \
    echo "ttf-mscorefonts-installer msttcorefonts/accepted-mscorefonts-eula select true" | debconf-set-selections && \
    apt-get install -y --no-install-recommends \
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
