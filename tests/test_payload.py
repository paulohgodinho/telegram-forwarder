from datetime import datetime, timedelta, timezone

from forwarder.payload import N8NPayload, format_date


def test_format_date_naive_treated_as_utc():
    dt = datetime(2026, 10, 24, 12, 0, 0)
    assert format_date(dt) == "2026-10-24T12:00:00Z"


def test_format_date_converts_non_utc_to_utc():
    tz = timezone(timedelta(hours=-3))
    dt = datetime(2026, 10, 24, 9, 0, 0, tzinfo=tz)
    assert format_date(dt) == "2026-10-24T12:00:00Z"


def test_format_date_drops_microseconds():
    dt = datetime(2026, 10, 24, 12, 0, 0, 123456, tzinfo=timezone.utc)
    assert format_date(dt) == "2026-10-24T12:00:00Z"


def test_to_dict_omits_empty_image_url():
    payload = N8NPayload(
        sender="Jane Doe",
        group_name="",
        text="hi",
        date=datetime(2026, 10, 24, tzinfo=timezone.utc),
    )
    data = payload.to_dict()

    assert "image_url" not in data
    assert list(data.keys()) == ["sender", "group_name", "text", "date"]


def test_to_dict_includes_image_url_when_present():
    payload = N8NPayload(
        sender="Jane Doe",
        group_name="Family Group",
        text="hi",
        date=datetime(2026, 10, 24, tzinfo=timezone.utc),
        image_url="https://example.com/photos/1.jpg",
    )
    data = payload.to_dict()

    assert data["image_url"] == "https://example.com/photos/1.jpg"
    assert list(data.keys()) == ["sender", "group_name", "text", "date", "image_url"]


def test_to_dict_values_match_go_shape():
    payload = N8NPayload(
        sender="Jane Doe",
        group_name="",
        text="hello",
        date=datetime(2026, 10, 24, 12, 30, 0, tzinfo=timezone.utc),
    )
    assert payload.to_dict() == {
        "sender": "Jane Doe",
        "group_name": "",
        "text": "hello",
        "date": "2026-10-24T12:30:00Z",
    }
