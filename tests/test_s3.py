from datetime import datetime, timezone
from unittest.mock import AsyncMock

import pytest

from forwarder.config import Config
from forwarder.s3 import UPLOAD_RETRIES, S3Uploader, photo_key


class _FakeClientCM:
    def __init__(self, client):
        self._client = client

    async def __aenter__(self):
        return self._client

    async def __aexit__(self, *exc_info):
        return False


class _FakeSession:
    def __init__(self, client):
        self._client = client

    def client(self, *args, **kwargs):
        return _FakeClientCM(self._client)


def _config(**overrides):
    base = dict(
        telegram_app_id=1,
        telegram_app_hash="hash",
        n8n_webhook_url="https://n8n.example.com",
        web_auth_port=56899,
        s3_endpoint_url="https://s3.example.com",
        s3_region="garage",
        s3_bucket="bucket",
        s3_access_key_id="key",
        s3_secret_access_key="secret",
        s3_public_base_url="https://public.example.com",
    )
    base.update(overrides)
    return Config(**base)


def test_photo_key_is_date_sharded():
    dt = datetime(2026, 10, 24, 12, 0, 0, tzinfo=timezone.utc)
    assert photo_key(123, dt) == "photos/2026/10/24/123.jpg"


def test_photo_key_normalizes_naive_datetime_as_utc():
    dt = datetime(2026, 1, 2, 3, 4, 5)
    assert photo_key(1, dt) == "photos/2026/01/02/1.jpg"


async def test_upload_photo_succeeds_first_try():
    uploader = S3Uploader(_config())
    fake_client = AsyncMock()
    uploader._session = _FakeSession(fake_client)

    url = await uploader.upload_photo("photos/2026/10/24/1.jpg", b"data")

    assert url == "https://public.example.com/photos/2026/10/24/1.jpg"
    fake_client.put_object.assert_awaited_once()


async def test_upload_photo_retries_then_succeeds(monkeypatch):
    uploader = S3Uploader(_config())
    fake_client = AsyncMock()
    fake_client.put_object.side_effect = [RuntimeError("boom"), RuntimeError("boom"), None]
    uploader._session = _FakeSession(fake_client)
    monkeypatch.setattr("forwarder.s3.asyncio.sleep", AsyncMock())

    url = await uploader.upload_photo("key.jpg", b"data")

    assert url == "https://public.example.com/key.jpg"
    assert fake_client.put_object.await_count == 3


async def test_upload_photo_raises_after_exhausting_retries(monkeypatch):
    uploader = S3Uploader(_config())
    fake_client = AsyncMock()
    fake_client.put_object.side_effect = RuntimeError("boom")
    uploader._session = _FakeSession(fake_client)
    monkeypatch.setattr("forwarder.s3.asyncio.sleep", AsyncMock())

    with pytest.raises(RuntimeError, match="boom"):
        await uploader.upload_photo("key.jpg", b"data")

    assert fake_client.put_object.await_count == UPLOAD_RETRIES
