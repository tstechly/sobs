FROM python:3.12-slim

LABEL maintainer="sobs"
LABEL description="Simple Observe – lightweight OpenTelemetry telemetry container"

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY app.py .
COPY templates/ templates/
COPY static/ static/

# Data directory (mount a volume here for persistence)
RUN mkdir -p /data
ENV SOBS_DATA_DIR=/data

# Expose default port
EXPOSE 4317

# Production server (gunicorn with gthread workers for async processing)
CMD gunicorn --worker-class gthread --workers ${GUNICORN_WORKERS:-2} --threads ${GUNICORN_THREADS:-4} --bind 0.0.0.0:${PORT:-4317} app:app
