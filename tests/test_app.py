import asyncio
import logging
import os
from pathlib import Path

import pytest

from forwarder import app
from forwarder.app import (
    check_session_dir_writable,
    install_exception_handler,
    wait_until_authorized,
)


async def test_install_exception_handler_logs_exception(caplog):
    caplog.set_level(logging.DEBUG)
    loop = asyncio.get_running_loop()
    install_exception_handler(loop)

    handler = loop.get_exception_handler()
    assert handler is not None
    handler(loop, {"exception": RuntimeError("boom"), "message": "task failed"})

    record = next(r for r in caplog.records if r.msg == "exception recovered")
    assert record.fields["where"] == "asyncio task"
    assert "boom" in record.fields["error"]


async def test_install_exception_handler_logs_message_without_exception(caplog):
    caplog.set_level(logging.DEBUG)
    loop = asyncio.get_running_loop()
    install_exception_handler(loop)

    handler = loop.get_exception_handler()
    handler(loop, {"message": "something went wrong"})

    assert any(r.msg == "something went wrong" for r in caplog.records)


class _FakeClient:
    def __init__(self, authorized_after: int):
        self._calls = 0
        self._authorized_after = authorized_after

    async def is_user_authorized(self) -> bool:
        self._calls += 1
        return self._calls >= self._authorized_after


async def test_wait_until_authorized_polls_until_true(monkeypatch):
    sleeps = []

    async def fake_sleep(seconds):
        sleeps.append(seconds)

    monkeypatch.setattr(app.asyncio, "sleep", fake_sleep)

    client = _FakeClient(authorized_after=3)
    await wait_until_authorized(client)

    assert client._calls == 3
    assert sleeps == [app.AUTH_POLL_INTERVAL_SECONDS, app.AUTH_POLL_INTERVAL_SECONDS]


async def test_wait_until_authorized_returns_immediately_if_already_authorized():
    client = _FakeClient(authorized_after=1)
    await wait_until_authorized(client)
    assert client._calls == 1


def test_check_session_dir_writable_passes_when_writable(tmp_path, monkeypatch):
    monkeypatch.setattr(app, "SESSION_DIR", tmp_path)
    check_session_dir_writable()  # must not raise
    assert not (tmp_path / ".write_test").exists()


def test_check_session_dir_writable_raises_actionable_message(tmp_path, monkeypatch):
    monkeypatch.setattr(app, "SESSION_DIR", tmp_path)

    def fail_write(self, *args, **kwargs):
        raise PermissionError("Permission denied")

    monkeypatch.setattr(Path, "write_text", fail_write)

    with pytest.raises(RuntimeError) as exc_info:
        check_session_dir_writable()

    message = str(exc_info.value)
    assert str(tmp_path) in message
    assert f"uid={os.getuid()}" in message
    assert f"gid={os.getgid()}" in message
    assert "chown -R" in message
