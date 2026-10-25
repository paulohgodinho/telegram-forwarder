"""Environment-variable configuration, validated eagerly at startup."""

from __future__ import annotations

import os
from dataclasses import dataclass


class ConfigError(Exception):
    """Raised when a required environment variable is missing or invalid."""


def _require(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if not value:
        raise ConfigError(f"{name} is required")
    return value


@dataclass(frozen=True)
class Config:
    telegram_app_id: int
    telegram_app_hash: str
    n8n_webhook_url: str
    web_auth_port: int

    s3_endpoint_url: str
    s3_region: str
    s3_bucket: str
    s3_access_key_id: str
    s3_secret_access_key: str
    s3_public_base_url: str

    @classmethod
    def from_env(cls) -> "Config":
        app_id_raw = _require("TELEGRAM_APP_ID")
        try:
            app_id = int(app_id_raw)
        except ValueError as exc:
            raise ConfigError("TELEGRAM_APP_ID must be an integer") from exc

        port_raw = os.environ.get("WEB_AUTH_PORT", "56899").strip() or "56899"
        try:
            port = int(port_raw)
        except ValueError as exc:
            raise ConfigError("WEB_AUTH_PORT must be an integer") from exc

        return cls(
            telegram_app_id=app_id,
            telegram_app_hash=_require("TELEGRAM_APP_HASH"),
            n8n_webhook_url=_require("TELEGRAM_N8N_WEBHOOK_URL"),
            web_auth_port=port,
            s3_endpoint_url=_require("S3_ENDPOINT_URL"),
            s3_region=_require("S3_REGION"),
            s3_bucket=_require("S3_BUCKET"),
            s3_access_key_id=_require("S3_ACCESS_KEY_ID"),
            s3_secret_access_key=_require("S3_SECRET_ACCESS_KEY"),
            s3_public_base_url=_require("S3_PUBLIC_BASE_URL").rstrip("/"),
        )
