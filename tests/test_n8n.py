import logging
from datetime import datetime, timezone

from forwarder.n8n import forward_to_n8n
from forwarder.payload import N8NPayload


class _FakeResponse:
    def __init__(self, status: int, body: str = ""):
        self.status = status
        self._body = body

    async def text(self) -> str:
        return self._body

    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc_info):
        return False


class _FakeSession:
    def __init__(self, response: _FakeResponse):
        self._response = response
        self.last_kwargs: dict | None = None

    def post(self, url, **kwargs):
        self.last_kwargs = kwargs
        return self._response


class _RaisingSession:
    def post(self, *args, **kwargs):
        raise RuntimeError("network down")


def _payload() -> N8NPayload:
    return N8NPayload(
        sender="Jane Doe",
        group_name="Family Group",
        text="hello there",
        date=datetime(2026, 10, 24, tzinfo=timezone.utc),
        image_url="https://example.com/photos/1.jpg",
    )


async def test_forward_to_n8n_success_logs_delivery_result_with_image_url(caplog):
    logger = logging.getLogger("test.n8n.success")
    session = _FakeSession(_FakeResponse(200, "ok"))

    with caplog.at_level(logging.INFO, logger="test.n8n.success"):
        await forward_to_n8n(session, logger, "https://hook.example.com", _payload())

    record = next(r for r in caplog.records if r.msg == "n8n delivery result")
    assert record.fields["status_code"] == 200
    assert record.fields["image_url"] == "https://example.com/photos/1.jpg"
    assert record.fields["sender"] == "Jane Doe"


async def test_forward_to_n8n_rejected_status_logs_error(caplog):
    logger = logging.getLogger("test.n8n.rejected")
    session = _FakeSession(_FakeResponse(500, "server error"))

    with caplog.at_level(logging.ERROR, logger="test.n8n.rejected"):
        await forward_to_n8n(session, logger, "https://hook.example.com", _payload())

    record = next(r for r in caplog.records if r.msg == "n8n webhook rejected request")
    assert record.fields["status_code"] == 500
    assert record.fields["image_url"] == "https://example.com/photos/1.jpg"
    assert record.fields["response_body"] == "server error"


async def test_forward_to_n8n_send_failure_logs_error(caplog):
    logger = logging.getLogger("test.n8n.exc")

    with caplog.at_level(logging.ERROR, logger="test.n8n.exc"):
        await forward_to_n8n(_RaisingSession(), logger, "https://hook.example.com", _payload())

    record = next(r for r in caplog.records if r.msg == "failed to send message to n8n")
    assert "network down" in record.fields["error"]
    assert record.fields["image_url"] == "https://example.com/photos/1.jpg"


async def test_forward_to_n8n_sends_payload_as_json_body():
    logger = logging.getLogger("test.n8n.body")
    session = _FakeSession(_FakeResponse(200, "ok"))

    await forward_to_n8n(session, logger, "https://hook.example.com", _payload())

    assert session.last_kwargs["json"] == {
        "sender": "Jane Doe",
        "group_name": "Family Group",
        "text": "hello there",
        "date": "2026-10-24T00:00:00Z",
        "image_url": "https://example.com/photos/1.jpg",
    }
