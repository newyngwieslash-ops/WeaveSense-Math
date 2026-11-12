# Math service image. Context is the repository root.
# The process only receives MATH_* variables; it must not inherit platform
# database, Redis, or updater secrets (hard constraint 7).
FROM python:3.12-slim

WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    MATH_DATA_DIR=/data \
    MATH_LISTEN=0.0.0.0:8790

COPY services/math-service/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt \
    && useradd --system --uid 10001 --home-dir /app --shell /usr/sbin/nologin mathsvc \
    && mkdir -p /data \
    && chown mathsvc:mathsvc /data

COPY --chown=mathsvc:mathsvc services/math-service/mathsvc /app/mathsvc
COPY --chown=mathsvc:mathsvc services/math-service/LICENSE-edulab services/math-service/NOTICE /app/

USER mathsvc
VOLUME ["/data"]
EXPOSE 8790

HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=10 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8790/healthz', timeout=2)"

CMD ["python", "-m", "mathsvc.server"]
