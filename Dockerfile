# syntax=docker/dockerfile:1

# Chainguard Wolfi-based: "latest-dev" (uv/pip/apk/shell) for building,
# "latest" (distroless, nonroot by default) for the actual runtime.
FROM cgr.dev/chainguard/python:latest-dev AS base
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
ENTRYPOINT ["uv", "run", "pytest"]

# ---- build: full venv (incl. project itself) + source + session dir,
# assembled here since the final runtime image has no shell/uv to do it ----
FROM deps AS build
COPY . .
RUN --mount=type=cache,target=/root/.cache/uv \
    mkdir -p session/data && uv sync --frozen --no-dev

# ---- runtime: distroless, no shell/package manager, nonroot by default ----
FROM cgr.dev/chainguard/python:latest AS runtime
WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /app /app
ENV PATH="/app/.venv/bin:${PATH}"
ENTRYPOINT ["python", "main.py"]
