"""Web-based Telegram login: phone -> code -> optional 2FA password."""

from __future__ import annotations

import logging

from aiohttp import web
from telethon import TelegramClient
from telethon.errors import SessionPasswordNeededError

from forwarder.logging_setup import error as log_error
from forwarder.logging_setup import info as log_info

logger = logging.getLogger("forwarder")

_PAGE_AUTHENTICATED = (
    "<h1>Authenticated</h1><p>Client is running and forwarding messages.</p>"
)

_PAGE_PASSWORD = """
<h2>Two-Step Verification Enabled</h2>
<form action="/password" method="post">
    Password: <input type="password" name="password" required>
    <input type="submit" value="Submit Password">
</form>"""

_PAGE_CODE = """
<h2>Enter Login Code</h2>
<form action="/code" method="post">
    Code: <input type="text" name="code" required>
    <input type="submit" value="Submit Code">
</form>"""

_PAGE_PHONE = """
<h2>Telegram Login</h2>
<form action="/phone" method="post">
    Phone Number (e.g. +1234567890): <input type="text" name="phone" required>
    <input type="submit" value="Send Code">
</form>"""


class WebAuth:
    """Holds login-flow state across the phone/code/password steps."""

    def __init__(self, client: TelegramClient) -> None:
        self._client = client
        self._phone: str | None = None
        self._phone_code_hash: str | None = None
        self._needs_password = False

    async def index(self, _request: web.Request) -> web.Response:
        try:
            authorized = await self._client.is_user_authorized()
        except Exception as exc:  # noqa: BLE001
            log_error(logger, "telegram is_user_authorized check failed", error=str(exc))
            authorized = False
        if authorized:
            return web.Response(text=_PAGE_AUTHENTICATED, content_type="text/html")
        if self._needs_password:
            html = _PAGE_PASSWORD
        elif self._phone_code_hash:
            html = _PAGE_CODE
        else:
            html = _PAGE_PHONE
        return web.Response(text=html, content_type="text/html")

    async def submit_phone(self, request: web.Request) -> web.Response:
        data = await request.post()
        phone = str(data.get("phone", "")).strip()
        if not phone:
            return web.Response(text="phone is required", status=400)

        try:
            sent = await self._client.send_code_request(phone)
        except Exception as exc:  # noqa: BLE001
            log_error(logger, "telegram send code request failed", error=str(exc))
            return web.Response(text=f"Error: {exc}", status=400)

        self._phone = phone
        self._phone_code_hash = sent.phone_code_hash
        log_info(logger, "telegram login code requested", phone=phone)
        raise web.HTTPFound("/")

    async def submit_code(self, request: web.Request) -> web.Response:
        data = await request.post()
        code = str(data.get("code", "")).strip()
        if not code or not self._phone or not self._phone_code_hash:
            return web.Response(text="phone and code are required", status=400)

        try:
            await self._client.sign_in(
                self._phone, code, phone_code_hash=self._phone_code_hash
            )
        except SessionPasswordNeededError:
            self._needs_password = True
        except Exception as exc:  # noqa: BLE001
            log_error(logger, "telegram sign-in with code failed", error=str(exc))
            return web.Response(text=f"Error: {exc}", status=400)
        else:
            log_info(logger, "telegram login succeeded")
        raise web.HTTPFound("/")

    async def submit_password(self, request: web.Request) -> web.Response:
        data = await request.post()
        password = str(data.get("password", ""))

        try:
            await self._client.sign_in(password=password)
        except Exception as exc:  # noqa: BLE001
            log_error(logger, "telegram 2fa sign-in failed", error=str(exc))
            return web.Response(text=f"Error: {exc}", status=400)
        log_info(logger, "telegram login succeeded (2fa)")
        raise web.HTTPFound("/")

    def routes(self) -> web.RouteTableDef:
        routes = web.RouteTableDef()
        routes.get("/")(self.index)
        routes.post("/phone")(self.submit_phone)
        routes.post("/code")(self.submit_code)
        routes.post("/password")(self.submit_password)
        return routes
