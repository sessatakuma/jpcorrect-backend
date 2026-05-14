FROM python:3.11-slim AS builder

WORKDIR /app

# Install uv for fast dependency management
RUN pip install --no-cache-dir uv

# Copy dependency files first for better layer caching
COPY pyproject.toml uv.lock ./

# Install dependencies using uv
RUN uv sync --frozen --no-dev --no-install-project

COPY . .

FROM python:3.11-slim

WORKDIR /app

# Install uv in runtime image
RUN pip install --no-cache-dir uv

# Copy installed dependencies and app from builder
COPY --from=builder /app/.venv /app/.venv
COPY --from=builder /app /app

# Set PATH to use venv binaries
ENV PATH="/app/.venv/bin:$PATH"
ENV PYTHONUNBUFFERED=1

EXPOSE 8000

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]