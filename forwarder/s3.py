"""S3-compatible (Garage) photo storage: client, key scheme, retried upload."""

from __future__ import annotations

import asyncio
import logging
from datetime import datetime, timezone

import aioboto3
from botocore.config import Config as BotoConfig

from forwarder.config import Config
from forwarder.logging_setup import error as log_error
from forwarder.logging_setup import warning as log_warning

UPLOAD_RETRIES = 3
RETRY_BASE_DELAY_SECONDS = 0.5
PHOTO_CONTENT_TYPE = "image/jpeg"

logger = logging.getLogger("forwarder")


def photo_key(photo_id: int, photo_date: datetime) -> str:
    """Builds the S3 key photos/YYYY/MM/DD/<photo_id>.jpg, matching the Go
    implementation's date-sharded scheme exactly.
    """
    date = photo_date.astimezone(timezone.utc) if photo_date.tzinfo else photo_date
    return f"photos/{date:%Y/%m/%d}/{photo_id}.jpg"


class S3Uploader:
    """Wraps an aioboto3 session, reusing one client for all uploads."""

    def __init__(self, config: Config) -> None:
        self._config = config
        self._session = aioboto3.Session()

    def _client_cm(self):
        return self._session.client(
            "s3",
            endpoint_url=self._config.s3_endpoint_url,
            region_name=self._config.s3_region,
            aws_access_key_id=self._config.s3_access_key_id,
            aws_secret_access_key=self._config.s3_secret_access_key,
            # Garage, like most self-hosted S3-compatible stores, only
            # supports path-style addressing, not virtual-hosted style.
            config=BotoConfig(s3={"addressing_style": "path"}),
        )

    def public_url(self, key: str) -> str:
        return f"{self._config.s3_public_base_url}/{key}"

    async def upload_photo(self, key: str, data: bytes) -> str:
        """Uploads data to key, retrying up to UPLOAD_RETRIES times, and
        returns the object's public URL on success.
        """
        last_exc: Exception | None = None
        for attempt in range(1, UPLOAD_RETRIES + 1):
            try:
                async with self._client_cm() as client:
                    await client.put_object(
                        Bucket=self._config.s3_bucket,
                        Key=key,
                        Body=data,
                        ContentType=PHOTO_CONTENT_TYPE,
                    )
                return self.public_url(key)
            except Exception as exc:  # noqa: BLE001 - want to retry on anything
                last_exc = exc
                log_warning(
                    logger,
                    "S3 upload attempt failed",
                    key=key,
                    attempt=attempt,
                    max_attempts=UPLOAD_RETRIES,
                    error=str(exc),
                )
                if attempt < UPLOAD_RETRIES:
                    await asyncio.sleep(RETRY_BASE_DELAY_SECONDS * attempt)

        log_error(
            logger,
            "S3 upload failed after all attempts",
            key=key,
            max_attempts=UPLOAD_RETRIES,
            error=str(last_exc),
        )
        raise last_exc  # type: ignore[misc]
