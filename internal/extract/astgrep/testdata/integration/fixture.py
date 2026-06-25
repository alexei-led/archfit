# Minimal Python fixture for per-rule-file integration tests.
# Covers: py-func, py-class, py-decorator,
#         py-route-decorator (confirmed via py-import-fastapi signal),
#         py-test-import-pytest (test_import fact).

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
