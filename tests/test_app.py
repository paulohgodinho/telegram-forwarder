import asyncio
import logging

from forwarder import app
from forwarder.app import install_exception_handler, wait_until_authorized


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
