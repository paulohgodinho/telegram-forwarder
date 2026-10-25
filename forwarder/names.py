"""Human-friendly display names for Telegram entities.

Ports the precedence from the Go implementation's peerDisplayName:
title/name first, then @username, then a generic kind+id fallback.
"""

from __future__ import annotations

from telethon.tl import types


def display_name(entity: object | None) -> str:
    """Returns a human-friendly name for a resolved Telethon entity."""
    if isinstance(entity, types.Channel):
        title = (entity.title or "").strip()
        if title:
            return title
        if entity.username:
            return f"@{entity.username}"
        return f"Channel({entity.id})"

    if isinstance(entity, types.Chat):
        title = (entity.title or "").strip()
        if title:
            return title
        return f"Chat({entity.id})"

    if isinstance(entity, types.User):
        name = f"{entity.first_name or ''} {entity.last_name or ''}".strip()
        if name:
            return name
        if entity.username:
            return f"@{entity.username}"
        return f"User({entity.id})"

    return "Unknown"
