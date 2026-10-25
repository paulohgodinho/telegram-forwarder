# syntax=docker/dockerfile:1

FROM ghcr.io/astral-sh/uv:python3.13-bookworm-slim AS base
WORKDIR /app
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    PYTHONUNBUFFERED=1
COPY pyproject.toml uv.lock ./

# ---- deps: prod-only dependencies, cached independently of app code ----
FROM base AS deps
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

# ---- test: dev deps + project code, runs the pytest suite ----
FROM base AS test
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-install-project
COPY . .
RUN --mount=type=cache,target=/root/.cache/uv uv sync --frozen
ENV PATH="/app/.venv/bin:${PATH}"
CMD ["uv", "run", "pytest"]

# ---- runtime: prod deps + project only, no dev deps, non-root user ----
FROM deps AS runtime
COPY . .
RUN --mount=type=cache,target=/root/.cache/uv uv sync --frozen --no-dev
RUN useradd --create-home --uid 1000 appuser \
    # Pre-create the volume mount point so Docker seeds a fresh named
    # volume as appuser-writable, not root:root.
    && mkdir -p /app/session/data \
    && chown -R appuser:appuser /app
USER appuser
ENV PATH="/app/.venv/bin:${PATH}"
ENTRYPOINT ["python", "main.py"]
