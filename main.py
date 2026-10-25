"""Entrypoint: gets messages from Telegram and forwards them to n8n."""

from __future__ import annotations

import asyncio
import logging

from forwarder import app
from forwarder.logging_setup import error as log_error
from forwarder.logging_setup import info as log_info
from forwarder.logging_setup import setup_logging


def main() -> None:
    n8n_logger = setup_logging(level=logging.INFO)
    logger = logging.getLogger("forwarder")
    try:
        asyncio.run(app.run(n8n_logger))
    except KeyboardInterrupt:
        log_info(logger, "terminated by user")
    except Exception as exc:  # noqa: BLE001 - top-level catch-all, logged then exit(1)
        log_error(logger, "runtime error", error=str(exc))
        raise SystemExit(1) from exc


if __name__ == "__main__":
    main()
