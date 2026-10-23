import archfit_fixture_not_installed
from modern314.adapter import value

try:
    from ._version import __version__
except ImportError:
    __version__ = "unknown"


def read():
    try:
        return value()
    except KeyError, ValueError:
        return 0
