import json

import pytest

import main as main_module


async def _raise(exc: BaseException):
    raise exc


def _parse_json_lines(out: str) -> list[dict]:
    """main() calls setup_logging(), which replaces root's handlers (and
    with it caplog's capture handler) - so these tests read real stdout.
    """
    records = []
    for line in out.strip().splitlines():
        if line.strip().startswith("{"):
            records.append(json.loads(line))
    return records


def test_main_logs_runtime_error_and_exits_1(monkeypatch, capsys):
    monkeypatch.setattr(
        main_module.app, "run", lambda n8n_logger: _raise(RuntimeError("boom"))
    )

    with pytest.raises(SystemExit) as exc_info:
        main_module.main()

    assert exc_info.value.code == 1
    records = _parse_json_lines(capsys.readouterr().out)
    record = next(r for r in records if r["msg"] == "runtime error")
    assert record["error"] == "boom"


def test_main_handles_keyboard_interrupt_cleanly(monkeypatch, capsys):
    monkeypatch.setattr(
        main_module.app, "run", lambda n8n_logger: _raise(KeyboardInterrupt())
    )

    main_module.main()  # must not raise

    records = _parse_json_lines(capsys.readouterr().out)
    assert any(r["msg"] == "terminated by user" for r in records)
