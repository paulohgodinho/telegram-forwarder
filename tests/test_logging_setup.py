import json
import logging

from forwarder.logging_setup import (
    N8N_LOGGER_NAME,
    CompactJSONFormatter,
    PrettyJSONFormatter,
    log_exception,
    setup_logging,
    truncate,
)


def _make_record(level=logging.INFO, msg="hello", fields=None):
    record = logging.LogRecord("test", level, __file__, 1, msg, (), None)
    if fields is not None:
        record.fields = fields
    return record


def test_truncate_shorter_than_limit():
    assert truncate("hi", 40) == "hi"


def test_truncate_exactly_at_limit():
    s = "a" * 40
    assert truncate(s, 40) == s


def test_truncate_longer_than_limit():
    assert truncate("a" * 50, 40) == "a" * 40


def test_truncate_default_length_is_40():
    assert truncate("a" * 50) == "a" * 40


def test_compact_formatter_is_single_line():
    fmt = CompactJSONFormatter()
    record = _make_record(fields={"channel": "Family Group", "message": "hi"})
    out = fmt.format(record)

    assert "\n" not in out
    decoded = json.loads(out)
    assert decoded["msg"] == "hello"
    assert decoded["level"] == "INFO"
    assert decoded["channel"] == "Family Group"
    assert decoded["message"] == "hi"


def test_compact_formatter_field_order_matches_call_order():
    fmt = CompactJSONFormatter()
    record = _make_record(fields={"b": 2, "a": 1})
    out = fmt.format(record)

    # time, level, msg always first, then fields in insertion order
    assert list(json.loads(out).keys()) == ["time", "level", "msg", "b", "a"]


def test_pretty_formatter_is_multiline_and_sorted():
    fmt = PrettyJSONFormatter()
    record = _make_record(
        level=logging.ERROR,
        msg="n8n delivery result",
        fields={"image_url": "https://example.com/x.jpg", "status_code": 200},
    )
    out = fmt.format(record)

    assert "\n" in out
    decoded = json.loads(out)
    assert decoded["image_url"] == "https://example.com/x.jpg"
    assert decoded["status_code"] == 200
    assert list(decoded.keys()) == sorted(decoded.keys())


def test_setup_logging_installs_compact_default_and_pretty_n8n_logger():
    n8n_logger = setup_logging(level=logging.INFO)

    root = logging.getLogger()
    assert isinstance(root.handlers[0].formatter, CompactJSONFormatter)

    assert n8n_logger.name == N8N_LOGGER_NAME
    assert n8n_logger.propagate is False
    assert isinstance(n8n_logger.handlers[0].formatter, PrettyJSONFormatter)


def test_log_exception_includes_where_error_and_stack(caplog):
    logger = logging.getLogger("test.exc")
    try:
        raise ValueError("boom")
    except ValueError as exc:
        with caplog.at_level(logging.ERROR, logger="test.exc"):
            log_exception(logger, "unit test", exc)

    record = caplog.records[-1]
    assert record.fields["where"] == "unit test"
    assert record.fields["error"] == "boom"
    assert "ValueError" in record.fields["stack"]
