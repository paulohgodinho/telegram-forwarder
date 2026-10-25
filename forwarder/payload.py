"""n8n webhook payload: byte-for-byte compatible with the Go implementation.

Field order and shape: {sender, group_name, text, date, image_url}, with
image_url omitted entirely when empty (Go's `json:",omitempty"`), and date
as RFC3339 UTC with a "Z" suffix (matching Go's time.Time JSON encoding).
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone


def format_date(dt: datetime) -> str:
    """Formats dt as RFC3339 UTC with a Z suffix, no fractional seconds."""
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    dt = dt.astimezone(timezone.utc).replace(microsecond=0)
    return dt.isoformat().replace("+00:00", "Z")


@dataclass
class N8NPayload:
    sender: str
    group_name: str
    text: str
    date: datetime
    image_url: str = ""

    def to_dict(self) -> dict:
        data = {
            "sender": self.sender,
            "group_name": self.group_name,
            "text": self.text,
            "date": format_date(self.date),
        }
        if self.image_url:
            data["image_url"] = self.image_url
        return data
