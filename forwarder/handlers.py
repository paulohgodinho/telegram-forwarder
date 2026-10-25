"""Telegram NewMessage handler: resolves names, uploads photos, forwards to n8n."""

from __future__ import annotations

import asyncio
import logging

import aiohttp
from telethon import TelegramClient, events

from forwarder.logging_setup import info as log_info
from forwarder.logging_setup import log_exception
from forwarder.logging_setup import truncate
from forwarder.n8n import forward_to_n8n
from forwarder.names import display_name
from forwarder.payload import N8NPayload
from forwarder.s3 import S3Uploader, photo_key

logger = logging.getLogger("forwarder")


def register_handlers(
    client: TelegramClient,
    uploader: S3Uploader,
    http_session: aiohttp.ClientSession,
    n8n_logger: logging.Logger,
    n8n_webhook_url: str,
) -> None:
    """Registers the NewMessage listener. Every message is isolated via
    safe_handle_message (Python equivalent of panic recovery).
    """

    async def _on_new_message(event: events.NewMessage.Event) -> None:
        await safe_handle_message(event, uploader, http_session, n8n_logger, n8n_webhook_url)

    client.add_event_handler(_on_new_message, events.NewMessage)


async def safe_handle_message(
    event: events.NewMessage.Event,
    uploader: S3Uploader,
    http_session: aiohttp.ClientSession,
    n8n_logger: logging.Logger,
    n8n_webhook_url: str,
) -> None:
    """Calls _handle_message, catching any exception so one bad message
    can't crash the client or affect any other message.
    """
    try:
        await _handle_message(event, uploader, http_session, n8n_logger, n8n_webhook_url)
    except Exception as exc:  # noqa: BLE001 - one bad message must not crash the handler
        log_exception(logger, "handling message", exc)


async def _handle_message(
    event: events.NewMessage.Event,
    uploader: S3Uploader,
    http_session: aiohttp.ClientSession,
    n8n_logger: logging.Logger,
    n8n_webhook_url: str,
) -> None:
    group_name = ""
    if not event.is_private:
        chat = await event.get_chat()
        group_name = display_name(chat)

    sender = await event.get_sender()
    sender_name = display_name(sender) if sender is not None else (group_name or "Unknown")

    image_url = ""
    if event.photo is not None:
        try:
            image_url = await _upload_photo(event, uploader)
        except Exception as exc:  # noqa: BLE001 - a failed upload must never block forwarding
            log_exception(logger, "upload photo", exc)

    payload = N8NPayload(
        sender=sender_name,
        group_name=group_name,
        text=event.raw_text or "",
        date=event.message.date,
        image_url=image_url,
    )

    log_info(
        logger,
        "received telegram message",
        channel=group_name,
        message=truncate(payload.text),
    )

    asyncio.create_task(
        forward_to_n8n(http_session, n8n_logger, n8n_webhook_url, payload)
    )


async def _upload_photo(event: events.NewMessage.Event, uploader: S3Uploader) -> str:
    photo = event.photo
    data = await event.download_media(bytes)
    if not data:
        raise RuntimeError("download_media returned no data")

    key = photo_key(photo.id, photo.date)
    url = await uploader.upload_photo(key, data)

    log_info(
        logger,
        "uploaded photo",
        key=key,
        url=url,
        photo_id=photo.id,
        bytes=len(data),
    )
    return url
