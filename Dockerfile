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

# Production server
CMD ["python", "-m", "flask", "--app", "app", "run", "--host", "0.0.0.0", "--port", "4317"]
