"""Forwards messages to the n8n webhook.

Logs are pretty (multi-line) JSON via a dedicated logger, and always
include image_url so the uploaded photo's URL is visible alongside the
delivery outcome.
"""

from __future__ import annotations

import http.client
import logging

import aiohttp

from forwarder.logging_setup import error as log_error
from forwarder.logging_setup import info as log_info
from forwarder.logging_setup import log_exception
from forwarder.payload import N8NPayload

REQUEST_TIMEOUT_SECONDS = 10


async def forward_to_n8n(
    session: aiohttp.ClientSession,
    n8n_logger: logging.Logger,
    url: str,
    payload: N8NPayload,
) -> None:
    """POSTs payload to url. Never raises - every failure is logged."""
    try:
        data = payload.to_dict()
    except Exception as exc:  # noqa: BLE001 - must never raise
        log_exception(n8n_logger, "marshal n8n payload", exc)
        return

    log_info(
        n8n_logger,
        "attempting to send message to n8n",
        url=url,
        sender=payload.sender,
        text=payload.text,
        date=data["date"],
        image_url=payload.image_url,
    )

    try:
        timeout = aiohttp.ClientTimeout(total=REQUEST_TIMEOUT_SECONDS)
        async with session.post(url, json=data, timeout=timeout) as resp:
            body = await resp.text()
            status_text = http.client.responses.get(resp.status, "")

            if resp.status >= 400:
                log_error(
                    n8n_logger,
                    "n8n webhook rejected request",
                    url=url,
                    status_code=resp.status,
                    status_text=status_text,
                    sender=payload.sender,
                    text=payload.text,
                    image_url=payload.image_url,
                    response_body=body,
                )
                return

            log_info(
                n8n_logger,
                "n8n delivery result",
                url=url,
                status_code=resp.status,
                status_text=status_text,
                sender=payload.sender,
                text=payload.text,
                image_url=payload.image_url,
                response_body=body,
            )
    except Exception as exc:  # noqa: BLE001 - must never raise
        log_error(
            n8n_logger,
            "failed to send message to n8n",
            url=url,
            sender=payload.sender,
            text=payload.text,
            image_url=payload.image_url,
            error=str(exc),
        )
