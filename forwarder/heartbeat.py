"""Periodic Telegram connectivity check, logged every attempt."""

from __future__ import annotations

import asyncio
import logging
import random

from telethon import TelegramClient
from telethon.tl.functions import PingRequest

from forwarder.logging_setup import error as log_error
from forwarder.logging_setup import info as log_info

HEARTBEAT_INTERVAL_SECONDS = 60
PING_TIMEOUT_SECONDS = 5

logger = logging.getLogger("forwarder")


async def run_heartbeat(client: TelegramClient) -> None:
    """Pings Telegram every HEARTBEAT_INTERVAL_SECONDS until cancelled,
    logging every attempt (success and failure).
    """
    while True:
        await asyncio.sleep(HEARTBEAT_INTERVAL_SECONDS)
        await ping_once(client)


async def ping_once(client: TelegramClient) -> None:
    """Performs a single bounded ping attempt and logs its outcome."""
    try:
        ping_id = random.getrandbits(63)
        await asyncio.wait_for(
            client(PingRequest(ping_id=ping_id)),
            timeout=PING_TIMEOUT_SECONDS,
        )
    except Exception as exc:  # noqa: BLE001 - log every failure, never raise
        log_error(logger, "telegram heartbeat failed", error=str(exc))
        return
    log_info(logger, "telegram heartbeat ok")
