"""Wires config, the Telegram client, S3 uploader, web auth, and handlers."""

from __future__ import annotations

import asyncio
import logging
import os
import signal
from pathlib import Path

import aiohttp
from aiohttp import web
from telethon import TelegramClient

from forwarder.config import Config
from forwarder.handlers import register_handlers
from forwarder.heartbeat import run_heartbeat
from forwarder.logging_setup import error as log_error
from forwarder.logging_setup import info as log_info
from forwarder.logging_setup import log_exception
from forwarder.s3 import S3Uploader
from forwarder.webauth import WebAuth

logger = logging.getLogger("forwarder")

SESSION_DIR = Path("session/data")
SESSION_NAME = "forwarder"
AUTH_POLL_INTERVAL_SECONDS = 1


def install_exception_handler(loop: asyncio.AbstractEventLoop) -> None:
    """Last-resort net: logs any exception that escapes an asyncio task
    instead of Python dumping it straight to stderr unformatted.
    """

    def handler(_loop: asyncio.AbstractEventLoop, context: dict) -> None:
        exc = context.get("exception")
        if exc is not None:
            log_exception(logger, "asyncio task", exc)
        else:
            log_error(logger, context.get("message", "unhandled asyncio exception"))

    loop.set_exception_handler(handler)


def install_signal_handlers(loop: asyncio.AbstractEventLoop, main_task: asyncio.Task) -> None:
    """Cancels main_task on SIGINT/SIGTERM. Cancelling directly (rather than
    relying on the library to notice a disconnect) guarantees a bounded
    shutdown even if an in-flight RPC never resolves after disconnecting.
    """

    def shutdown() -> None:
        log_info(logger, "shutdown signal received")
        main_task.cancel()

    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, shutdown)


def check_session_dir_writable() -> None:
    """Fails fast with an actionable message instead of a cryptic SQLite
    error if SESSION_DIR's ownership doesn't match the current user - e.g.
    after switching base images to one with a different nonroot UID.
    """
    probe = SESSION_DIR / ".write_test"
    try:
        probe.write_text("ok")
        probe.unlink()
    except OSError as exc:
        st = SESSION_DIR.stat()
        raise RuntimeError(
            f"{SESSION_DIR} is owned by uid={st.st_uid}/gid={st.st_gid} but this "
            f"process runs as uid={os.getuid()}/gid={os.getgid()} - the session "
            "volume likely predates a base image change. Fix with: docker run "
            "--rm -v <volume_name>:/data alpine chown -R "
            f"{os.getuid()}:{os.getgid()} /data"
        ) from exc


async def wait_until_authorized(client: TelegramClient) -> None:
    """Blocks until the web login flow completes. run_until_disconnected()
    internally calls GetStateRequest(), which requires a logged-in session -
    calling it before login finishes raises AuthKeyUnregisteredError.
    """
    while not await client.is_user_authorized():
        await asyncio.sleep(AUTH_POLL_INTERVAL_SECONDS)


async def run(n8n_logger: logging.Logger) -> None:
    config = Config.from_env()
    SESSION_DIR.mkdir(parents=True, exist_ok=True)
    check_session_dir_writable()

    loop = asyncio.get_running_loop()
    install_exception_handler(loop)
    install_signal_handlers(loop, asyncio.current_task())

    client = TelegramClient(
        str(SESSION_DIR / SESSION_NAME),
        config.telegram_app_id,
        config.telegram_app_hash,
        catch_up=True,
        base_logger="telethon",
    )
    uploader = S3Uploader(config)
    heartbeat_task: asyncio.Task | None = None
    runner: web.AppRunner | None = None

    try:
        async with aiohttp.ClientSession() as http_session:
            register_handlers(
                client, uploader, http_session, n8n_logger, config.n8n_webhook_url
            )

            log_info(logger, "connecting to telegram")
            await client.connect()

            web_auth = WebAuth(client)
            web_app = web.Application()
            web_app.add_routes(web_auth.routes())
            runner = web.AppRunner(web_app)
            await runner.setup()
            site = web.TCPSite(runner, "0.0.0.0", config.web_auth_port)
            await site.start()
            log_info(logger, "web auth server listening", port=config.web_auth_port)

            if not await client.is_user_authorized():
                log_info(logger, "waiting for login via the web form")
                await wait_until_authorized(client)
                log_info(logger, "telegram login completed")

            # Telegram only pushes updates after an authenticated
            # high-level request; this also satisfies that requirement.
            await client.get_me()

            heartbeat_task = asyncio.create_task(run_heartbeat(client))

            log_info(logger, "listener running, forwarding incoming messages to n8n")
            await client.run_until_disconnected()
    except asyncio.CancelledError:
        log_info(logger, "shutting down")
    finally:
        if heartbeat_task is not None:
            heartbeat_task.cancel()
            try:
                await heartbeat_task
            except asyncio.CancelledError:
                pass
        try:
            await client.disconnect()
        except Exception as exc:  # noqa: BLE001 - shutdown must proceed either way
            log_exception(logger, "disconnect", exc)
        if runner is not None:
            await runner.cleanup()
