# Minimal Python fixture for per-rule-file integration tests.
# Covers: py-func, py-class, py-decorator,
#         py-route-decorator (confirmed via py-import-fastapi signal),
#         py-test-import-pytest (test_import fact),
#         py-lazy-import-module + py-lazy-import-from (lazy_import facts).

import pytest
from fastapi import FastAPI

app = FastAPI()


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


@app.get('/items')
def list_items():
    """FastAPI route — confirmed by import above."""
    return []


# Lazy-import fixtures: in-function imports must produce lazy_import facts.
# The top-level `import pytest` and `from fastapi import FastAPI` above must NOT
# match (they are module-level, not inside a function body).
def lazy_loader():
    """Function with a deferred module import — must produce a lazy_import fact."""
    import os  # lazy: import_statement inside function_definition
    return os.getcwd()


def lazy_from_loader():
    """Function with a deferred from-import — must produce a lazy_import fact."""
    from pathlib import Path  # lazy: import_from_statement inside function_definition
    return Path(".")
