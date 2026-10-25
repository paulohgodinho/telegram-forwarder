"""Structured JSON logging.

Mirrors the Go implementation: a compact single-line JSON logger is
installed as the root logger (so our own logs AND Telethon's internal
diagnostics both flow through it), while a separate pretty (multi-line)
JSON logger is used only for n8n delivery logs.
"""

from __future__ import annotations

import json
import logging
import sys
import traceback
from datetime import datetime, timezone

MESSAGE_PREVIEW_LEN = 40

N8N_LOGGER_NAME = "forwarder.n8n"


def truncate(s: str, n: int = MESSAGE_PREVIEW_LEN) -> str:
    """Returns the first n characters of s, unchanged if already shorter."""
    return s[:n]


def _record_time(record: logging.LogRecord) -> str:
    dt = datetime.fromtimestamp(record.created, tz=timezone.utc)
    return dt.isoformat().replace("+00:00", "Z")


class CompactJSONFormatter(logging.Formatter):
    """Single-line JSON per record, field order matching call order."""

    def format(self, record: logging.LogRecord) -> str:
        payload: dict = {
            "time": _record_time(record),
            "level": record.levelname,
            "msg": record.getMessage(),
        }
        fields = getattr(record, "fields", None)
        if fields:
            payload.update(fields)
        if record.exc_info:
            payload["exc_info"] = self.formatException(record.exc_info)
        return json.dumps(payload, default=str)


class PrettyJSONFormatter(logging.Formatter):
    """Indented, alphabetically-sorted JSON per record (multi-line)."""

    def format(self, record: logging.LogRecord) -> str:
        payload: dict = {
            "time": _record_time(record),
            "level": record.levelname,
            "msg": record.getMessage(),
        }
        fields = getattr(record, "fields", None)
        if fields:
            payload.update(fields)
        if record.exc_info:
            payload["exc_info"] = self.formatException(record.exc_info)
        return json.dumps(payload, indent=2, sort_keys=True, default=str)


def _install_handler(logger: logging.Logger, formatter: logging.Formatter) -> None:
    for h in list(logger.handlers):
        logger.removeHandler(h)
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(formatter)
    logger.addHandler(handler)


def setup_logging(level: int = logging.INFO) -> logging.Logger:
    """Installs the compact handler on the root logger and returns a
    separate pretty-JSON logger reserved for n8n delivery logs.
    """
    root = logging.getLogger()
    root.setLevel(level)
    _install_handler(root, CompactJSONFormatter())

    n8n_logger = logging.getLogger(N8N_LOGGER_NAME)
    n8n_logger.setLevel(level)
    n8n_logger.propagate = False
    _install_handler(n8n_logger, PrettyJSONFormatter())
    return n8n_logger


def info(logger: logging.Logger, msg: str, **fields) -> None:
    logger.info(msg, extra={"fields": fields})


def warning(logger: logging.Logger, msg: str, **fields) -> None:
    logger.warning(msg, extra={"fields": fields})


def error(logger: logging.Logger, msg: str, **fields) -> None:
    logger.error(msg, extra={"fields": fields})


def debug(logger: logging.Logger, msg: str, **fields) -> None:
    logger.debug(msg, extra={"fields": fields})


def log_exception(logger: logging.Logger, where: str, exc: BaseException) -> None:
    """Logs a caught exception with a full traceback, tagged with where."""
    error(
        logger,
        "exception recovered",
        where=where,
        error=str(exc),
        stack=traceback.format_exc(),
    )
