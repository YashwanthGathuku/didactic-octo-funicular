# Python FastAPI SentinelFlow AI Tier
FROM python:3.11-slim

WORKDIR /app

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

RUN groupadd -r sentinel && useradd -r -g sentinel sentinel && \
    mkdir -p /app && \
    chown -R sentinel:sentinel /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY . ./

USER sentinel

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')" || exit 1

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
