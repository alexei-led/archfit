# Minimal Python fixture for per-rule-file integration tests.
# Covers: py-func, py-class, py-decorator.


def process(data):
    """Public function — must appear in Syntax() facts."""
    return data


class Processor:
    """Public class."""
    pass


@staticmethod
def _hidden():
    """Private — must not appear in exported facts."""
    pass
