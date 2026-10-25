import logging

from forwarder.heartbeat import ping_once


class _FakeClient:
    def __init__(self, raise_exc: Exception | None = None):
        self._raise_exc = raise_exc
        self.calls = 0

    async def __call__(self, request):
        self.calls += 1
        if self._raise_exc is not None:
            raise self._raise_exc
        return object()


async def test_ping_once_logs_success(caplog):
    client = _FakeClient()
    with caplog.at_level(logging.INFO, logger="forwarder"):
        await ping_once(client)

    assert client.calls == 1
    assert any(r.msg == "telegram heartbeat ok" for r in caplog.records)


async def test_ping_once_logs_failure(caplog):
    client = _FakeClient(raise_exc=RuntimeError("connection reset"))
    with caplog.at_level(logging.ERROR, logger="forwarder"):
        await ping_once(client)

    record = next(r for r in caplog.records if r.msg == "telegram heartbeat failed")
    assert "connection reset" in record.fields["error"]
