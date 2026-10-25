from telethon.tl import types

from forwarder.names import display_name


def _photo():
    return types.ChatPhotoEmpty()


def test_channel_with_title():
    ch = types.Channel(id=3195745015, title="My Cool Channel", photo=_photo(), date=0)
    assert display_name(ch) == "My Cool Channel"


def test_channel_no_title_falls_back_to_username():
    ch = types.Channel(id=42, title="", photo=_photo(), date=0, username="coolchannel")
    assert display_name(ch) == "@coolchannel"


def test_channel_no_title_or_username_falls_back_to_generic():
    ch = types.Channel(id=42, title="", photo=_photo(), date=0)
    assert display_name(ch) == "Channel(42)"


def test_chat_with_title():
    chat = types.Chat(
        id=7, title="Family Group", photo=_photo(), participants_count=2, date=0, version=0
    )
    assert display_name(chat) == "Family Group"


def test_chat_no_title_falls_back_to_generic():
    chat = types.Chat(id=7, title="", photo=_photo(), participants_count=2, date=0, version=0)
    assert display_name(chat) == "Chat(7)"


def test_user_with_first_and_last_name():
    user = types.User(id=99, first_name="Jane", last_name="Doe", username="janedoe")
    assert display_name(user) == "Jane Doe"


def test_user_with_only_username():
    user = types.User(id=100, username="ghostuser")
    assert display_name(user) == "@ghostuser"


def test_user_with_no_name_info_falls_back_to_generic():
    user = types.User(id=101)
    assert display_name(user) == "User(101)"


def test_none_entity_falls_back_to_unknown():
    assert display_name(None) == "Unknown"


def test_unrecognized_entity_falls_back_to_unknown():
    assert display_name(object()) == "Unknown"
