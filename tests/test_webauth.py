from types import SimpleNamespace

from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer
from telethon.errors import SessionPasswordNeededError

from forwarder.webauth import WebAuth


class FakeTelegramClient:
    def __init__(self, authorized=False):
        self._authorized = authorized
        self.sent_phone = None
        self.sign_in_calls = []

    async def is_user_authorized(self):
        return self._authorized

    async def send_code_request(self, phone):
        self.sent_phone = phone
        return SimpleNamespace(phone_code_hash="hash123")

    async def sign_in(self, *args, **kwargs):
        self.sign_in_calls.append((args, kwargs))
        return SimpleNamespace()


class TwoFAClient(FakeTelegramClient):
    async def sign_in(self, *args, **kwargs):
        if kwargs.get("password") is not None:
            self.sign_in_calls.append((args, kwargs))
            return SimpleNamespace()
        raise SessionPasswordNeededError(request=None)


async def _make_client(telegram_client):
    web_auth = WebAuth(telegram_client)
    app = web.Application()
    app.add_routes(web_auth.routes())
    client = TestClient(TestServer(app))
    await client.start_server()
    return client


async def test_index_shows_phone_form_initially():
    client = await _make_client(FakeTelegramClient(authorized=False))
    try:
        resp = await client.get("/")
        text = await resp.text()
        assert resp.status == 200
        assert "Telegram Login" in text
    finally:
        await client.close()


async def test_index_shows_authenticated_page():
    client = await _make_client(FakeTelegramClient(authorized=True))
    try:
        resp = await client.get("/")
        text = await resp.text()
        assert "Authenticated" in text
    finally:
        await client.close()


async def test_phone_then_code_flow_progresses_and_signs_in():
    tg = FakeTelegramClient(authorized=False)
    client = await _make_client(tg)
    try:
        resp = await client.post(
            "/phone", data={"phone": "+1234567890"}, allow_redirects=False
        )
        assert resp.status == 302
        assert tg.sent_phone == "+1234567890"

        index = await client.get("/")
        assert "Enter Login Code" in await index.text()

        resp2 = await client.post("/code", data={"code": "12345"}, allow_redirects=False)
        assert resp2.status == 302
        assert tg.sign_in_calls[-1][0] == ("+1234567890", "12345")
    finally:
        await client.close()


async def test_phone_missing_returns_400():
    client = await _make_client(FakeTelegramClient())
    try:
        resp = await client.post("/phone", data={})
        assert resp.status == 400
    finally:
        await client.close()


async def test_code_missing_phone_returns_400():
    client = await _make_client(FakeTelegramClient())
    try:
        resp = await client.post("/code", data={"code": "12345"})
        assert resp.status == 400
    finally:
        await client.close()


async def test_code_requiring_2fa_shows_password_form_then_succeeds():
    tg = TwoFAClient(authorized=False)
    client = await _make_client(tg)
    try:
        await client.post("/phone", data={"phone": "+1234567890"}, allow_redirects=False)
        await client.post("/code", data={"code": "12345"}, allow_redirects=False)

        index = await client.get("/")
        assert "Two-Step Verification" in await index.text()

        resp = await client.post("/password", data={"password": "secret"}, allow_redirects=False)
        assert resp.status == 302
        assert tg.sign_in_calls[-1][1] == {"password": "secret"}
    finally:
        await client.close()
