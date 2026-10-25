import asyncio
import logging

from forwarder.app import install_exception_handler


async def test_install_exception_handler_logs_exception(caplog):
    caplog.set_level(logging.DEBUG)
    loop = asyncio.get_running_loop()
    install_exception_handler(loop)

    handler = loop.get_exception_handler()
    assert handler is not None
    handler(loop, {"exception": RuntimeError("boom"), "message": "task failed"})

    record = next(r for r in caplog.records if r.msg == "exception recovered")
    assert record.fields["where"] == "asyncio task"
    assert "boom" in record.fields["error"]


async def test_install_exception_handler_logs_message_without_exception(caplog):
    caplog.set_level(logging.DEBUG)
    loop = asyncio.get_running_loop()
    install_exception_handler(loop)

    handler = loop.get_exception_handler()
    handler(loop, {"message": "something went wrong"})

    assert any(r.msg == "something went wrong" for r in caplog.records)
