"""Store path segments must be collision-free.

The previous scheme replaced every unsafe character with "_", which is not
injective: two distinct identities could map to the same directory and would
then share one WhatsApp account.
"""

import base64
import importlib.util
import os

_SPEC = importlib.util.spec_from_file_location(
    "run_server",
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "run_server.py"),
)
run_server = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(run_server)

_path_segment = run_server._path_segment


def test_ids_differing_only_in_punctuation_do_not_collide():
    # The exact pair the old sanitiser collapsed together.
    assert _path_segment("adam@x.com") != _path_segment("adam.x.com")


def test_hex_ids_are_left_alone():
    """Existing store directories must keep their names — no migration."""
    user_id = "77f00f25ead04b3b90c67898431154dd"
    assert _path_segment(user_id) == user_id


def test_safe_ids_pass_through_unchanged():
    for value in ("simple", "with-dash", "with_underscore", "MiXed123"):
        assert _path_segment(value) == value


def test_unsafe_ids_are_encoded_and_reversible():
    encoded = _path_segment("adam@x.com")
    assert encoded.startswith("b~")
    padded = encoded[2:] + "=" * (-len(encoded[2:]) % 4)
    assert base64.urlsafe_b64decode(padded).decode() == "adam@x.com"


def test_a_separator_cannot_survive_encoding():
    """A "/" must never reach the path as a separator."""
    assert "/" not in _path_segment("a/b")
    assert _path_segment("a/b") != _path_segment("a_b")


def test_encoded_and_passthrough_namespaces_are_disjoint():
    """A raw id must not be able to impersonate an encoded one."""
    # "~" is not in the safe set, so a passthrough value can never start with
    # the prefix — meaning this literal gets encoded rather than passed through.
    assert _path_segment("b~YWRhbUB4LmNvbQ") != "b~YWRhbUB4LmNvbQ"


def test_distinct_inputs_stay_distinct():
    values = [
        "77f00f25ead04b3b90c67898431154dd",
        "adam@x.com",
        "adam.x.com",
        "adam_x_com",
        "a/b",
        "a_b",
        "b~x",
        "",
        "default",
    ]
    segments = [_path_segment(v) for v in values]
    assert len(set(segments)) == len(values)


def test_empty_id_still_produces_a_segment():
    # Must not return "", which would collapse into the parent directory.
    assert _path_segment("") not in ("", ".", "..")


def _make_pre_profiles_store(user_dir: str) -> None:
    """A store as it looked before profiles: dbs and per-chat media directories."""
    os.makedirs(user_dir, exist_ok=True)
    for name in ("whatsapp.db", "messages.db", "whatsapp.db-wal"):
        with open(os.path.join(user_dir, name), "w") as f:
            f.write(name)

    os.makedirs(os.path.join(user_dir, "12345@g.us"), exist_ok=True)
    with open(os.path.join(user_dir, "12345@g.us", "pic.jpg"), "w") as f:
        f.write("media")


def test_an_existing_session_is_moved_into_its_profile(tmp_path):
    """Otherwise the bridge finds an empty directory and asks to pair again.

    That is exactly what happened deploying this: every account has a default
    profile, so the new path segment orphaned the existing session.
    """
    user_dir = os.path.join(str(tmp_path), "77f00f25")
    _make_pre_profiles_store(user_dir)
    store_dir = os.path.join(user_dir, "default")

    run_server._migrate_pre_profile_store(user_dir, store_dir)

    assert os.path.isfile(os.path.join(store_dir, "whatsapp.db"))
    # Media moves too — it is the same account's data.
    assert os.path.isfile(os.path.join(store_dir, "12345@g.us", "pic.jpg"))
    assert not os.path.exists(os.path.join(user_dir, "whatsapp.db"))


def test_migration_does_not_run_twice(tmp_path):
    user_dir = os.path.join(str(tmp_path), "77f00f25")
    _make_pre_profiles_store(user_dir)
    store_dir = os.path.join(user_dir, "default")

    run_server._migrate_pre_profile_store(user_dir, store_dir)
    # A second profile must not swallow the first one's data.
    other = os.path.join(user_dir, "pab12")
    run_server._migrate_pre_profile_store(user_dir, other)

    assert os.path.isfile(os.path.join(store_dir, "whatsapp.db"))
    assert not os.path.exists(other)


def test_nothing_happens_without_a_pre_profiles_store(tmp_path):
    user_dir = os.path.join(str(tmp_path), "fresh")
    os.makedirs(user_dir, exist_ok=True)
    store_dir = os.path.join(user_dir, "default")

    run_server._migrate_pre_profile_store(user_dir, store_dir)

    # A new user starts clean rather than getting an empty profile directory.
    assert not os.path.exists(store_dir)
