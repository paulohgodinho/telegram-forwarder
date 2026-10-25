import pytest

from forwarder.config import Config, ConfigError

REQUIRED = {
    "TELEGRAM_APP_ID": "12345",
    "TELEGRAM_APP_HASH": "hash",
    "TELEGRAM_N8N_WEBHOOK_URL": "https://n8n.example.com/webhook",
    "S3_ENDPOINT_URL": "https://s3.example.com",
    "S3_REGION": "garage",
    "S3_BUCKET": "bucket",
    "S3_ACCESS_KEY_ID": "key",
    "S3_SECRET_ACCESS_KEY": "secret",
    "S3_PUBLIC_BASE_URL": "https://public.example.com/",
}


def _set_env(monkeypatch, overrides=None):
    overrides = overrides or {}
    for key, value in REQUIRED.items():
        monkeypatch.setenv(key, overrides.get(key, value))
    for key, value in overrides.items():
        if key not in REQUIRED:
            monkeypatch.setenv(key, value)


def test_from_env_happy_path(monkeypatch):
    _set_env(monkeypatch)
    cfg = Config.from_env()

    assert cfg.telegram_app_id == 12345
    assert cfg.telegram_app_hash == "hash"
    assert cfg.web_auth_port == 56899
    # trailing slash stripped
    assert cfg.s3_public_base_url == "https://public.example.com"


@pytest.mark.parametrize("missing", list(REQUIRED.keys()))
def test_from_env_missing_required_raises(monkeypatch, missing):
    _set_env(monkeypatch)
    monkeypatch.delenv(missing, raising=False)
    with pytest.raises(ConfigError):
        Config.from_env()


def test_from_env_invalid_app_id_raises(monkeypatch):
    _set_env(monkeypatch, {"TELEGRAM_APP_ID": "notanumber"})
    with pytest.raises(ConfigError):
        Config.from_env()


def test_from_env_custom_port(monkeypatch):
    _set_env(monkeypatch, {"WEB_AUTH_PORT": "9999"})
    cfg = Config.from_env()
    assert cfg.web_auth_port == 9999


def test_from_env_invalid_port_raises(monkeypatch):
    _set_env(monkeypatch, {"WEB_AUTH_PORT": "notaport"})
    with pytest.raises(ConfigError):
        Config.from_env()
