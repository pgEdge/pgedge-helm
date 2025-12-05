# Stage 1: Build dependencies
FROM python:3.11-slim AS builder

# Install build dependencies and compile Python packages
RUN pip install --no-cache-dir --prefix=/install \
    psycopg2-binary==2.9.9 \
    kubernetes==28.1.0 \
    PyYAML==6.0.1

# Stage 2: Runtime image using distroless
FROM gcr.io/distroless/python3-debian12:nonroot

# Copy Python packages from builder
COPY --from=builder /install /usr/local

# Copy the init-spock script into the image
COPY --chown=65532:65532 scripts/init-spock.py /scripts/init-spock.py

# Create necessary directories with proper permissions
# Note: distroless runs as nonroot user (UID 65532) by default
USER 65532

WORKDIR /scripts

# Set Python path to find installed packages
ENV PYTHONPATH=/usr/local/lib/python3.11/site-packages

# Default command
ENTRYPOINT ["python3"]
CMD ["--version"]
