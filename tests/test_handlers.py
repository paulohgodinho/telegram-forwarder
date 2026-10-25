import asyncio
import logging
from datetime import datetime, timezone
from types import SimpleNamespace
from unittest.mock import AsyncMock

from telethon.tl import types

from forwarder.handlers import _handle_message, safe_handle_message


class FakeEvent:
    def __init__(
        self,
        *,
        is_private=True,
        chat=None,
        sender=None,
        photo=None,
        text="hello",
        date=None,
        media_bytes=b"imgdata",
    ):
        self.is_private = is_private
        self.photo = photo
        self.raw_text = text
        self.message = SimpleNamespace(date=date or datetime(2026, 10, 24, tzinfo=timezone.utc))
        self.get_chat = AsyncMock(return_value=chat)
        self.get_sender = AsyncMock(return_value=sender)
        self.download_media = AsyncMock(return_value=media_bytes)


class FakeUploader:
    def __init__(self, url="https://public.example.com/photos/2026/10/24/1.jpg", raise_exc=None):
        self._url = url
        self._raise_exc = raise_exc
        self.calls = []

    async def upload_photo(self, key, data):
        self.calls.append((key, data))
        if self._raise_exc:
            raise self._raise_exc
        return self._url


def _user(id=99, first_name="Jane", last_name="Doe"):
    return types.User(id=id, first_name=first_name, last_name=last_name)


def _channel(id=1, title="Family Group"):
    return types.Channel(id=id, title=title, photo=types.ChatPhotoEmpty(), date=0)


def _photo(id=555, date=None):
    return SimpleNamespace(id=id, date=date or datetime(2026, 10, 24, tzinfo=timezone.utc))


async def _drain_tasks():
    """Lets any asyncio.create_task'd coroutine (the n8n forward) finish,
    avoiding 'task was destroyed but it is pending' noise in test output.
    """
    await asyncio.sleep(0)
    pending = [t for t in asyncio.all_tasks() if t is not asyncio.current_task()]
    if pending:
        await asyncio.gather(*pending, return_exceptions=True)


async def test_handle_message_private_chat_has_no_channel_name(caplog):
    caplog.set_level(logging.DEBUG)
    event = FakeEvent(is_private=True, sender=_user(), text="hi there")

    await _handle_message(event, FakeUploader(), object(), logging.getLogger("t"), "https://hook")
    await _drain_tasks()

    record = next(r for r in caplog.records if r.msg == "received telegram message")
    assert record.fields["channel"] == ""
    assert record.fields["message"] == "hi there"


async def test_handle_message_group_chat_sets_channel_name(caplog):
    caplog.set_level(logging.DEBUG)
    event = FakeEvent(is_private=False, chat=_channel(title="Family Group"), sender=_user())

    await _handle_message(event, FakeUploader(), object(), logging.getLogger("t"), "https://hook")
    await _drain_tasks()

    record = next(r for r in caplog.records if r.msg == "received telegram message")
    assert record.fields["channel"] == "Family Group"


async def test_handle_message_truncates_long_text(caplog):
    caplog.set_level(logging.DEBUG)
    event = FakeEvent(is_private=True, sender=_user(), text="a" * 100)

    await _handle_message(event, FakeUploader(), object(), logging.getLogger("t"), "https://hook")
    await _drain_tasks()

    record = next(r for r in caplog.records if r.msg == "received telegram message")
    assert record.fields["message"] == "a" * 40


async def test_handle_message_no_sender_does_not_raise(caplog):
    caplog.set_level(logging.DEBUG)
    event = FakeEvent(is_private=False, chat=_channel(title="Announcements"), sender=None)

    await _handle_message(event, FakeUploader(), object(), logging.getLogger("t"), "https://hook")
    await _drain_tasks()

    assert any(r.msg == "received telegram message" for r in caplog.records)


async def test_handle_message_with_photo_uploads_and_sets_image_url(caplog):
    caplog.set_level(logging.DEBUG)
    photo = _photo(id=777)
    event = FakeEvent(is_private=True, sender=_user(), photo=photo)
    uploader = FakeUploader(url="https://public.example.com/photos/2026/10/24/777.jpg")

    await _handle_message(event, uploader, object(), logging.getLogger("t"), "https://hook")
    await _drain_tasks()

    assert uploader.calls
    key, data = uploader.calls[0]
    assert key == "photos/2026/10/24/777.jpg"
    assert data == b"imgdata"

    upload_record = next(r for r in caplog.records if r.msg == "uploaded photo")
    assert upload_record.fields["photo_id"] == 777


async def test_handle_message_upload_failure_does_not_block_forwarding(caplog):
    caplog.set_level(logging.DEBUG)
    photo = _photo(id=1)
    event = FakeEvent(is_private=True, sender=_user(), photo=photo)
    uploader = FakeUploader(raise_exc=RuntimeError("s3 down"))

    await _handle_message(event, uploader, object(), logging.getLogger("t"), "https://hook")
    await _drain_tasks()

    assert any(
        r.msg == "exception recovered" and r.fields["where"] == "upload photo"
        for r in caplog.records
    )
    assert any(r.msg == "received telegram message" for r in caplog.records)


async def test_safe_handle_message_recovers_exceptions(caplog):
    caplog.set_level(logging.DEBUG)

    class ExplodingEvent:
        @property
        def is_private(self):
            raise RuntimeError("boom")

    await safe_handle_message(
        ExplodingEvent(), FakeUploader(), object(), logging.getLogger("t"), "https://hook"
    )

    record = next(r for r in caplog.records if r.msg == "exception recovered")
    assert record.fields["where"] == "handling message"
    assert "boom" in record.fields["error"]
