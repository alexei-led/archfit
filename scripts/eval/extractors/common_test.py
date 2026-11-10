import unittest

from . import common


class GlobMatchTest(unittest.TestCase):
    def test_doublestar_cases(self):
        cases = [
            ("**/fonts/**", "src/ccgram/fonts/a.ttf", True),
            ("**/fonts/**", "fonts", True),
            ("**/fonts/**", "fontsx/a", False),
            ("internal/a/**", "internal/a", True),
            ("internal/a/**", "internal/a/b/c.go", True),
            ("internal/a/**", "internal/ab", False),
            ("*.py", "a/b.py", False),
            ("**/*.py", "a/b.py", True),
            ("**/*.py", "b.py", True),
            ("a/?.go", "a/b.go", True),
            ("a/?.go", "a/bc.go", False),
            ("**/.archfit-cache/**", "x/.archfit-cache/y", True),
            ("internal/gen", "internal/gen/x", False),
        ]
        for pattern, path, want in cases:
            with self.subTest(pattern=pattern, path=path):
                self.assertEqual(common.glob_match(pattern, path), want)


class FactsTest(unittest.TestCase):
    def test_sorted_and_deduplicated(self):
        got = common.facts(
            "python",
            ["b", "a", "a"],
            [("b", "c"), ("a", "c"), ("a", "c")],
            [("a", "x")],
            2,
        )
        self.assertEqual(got["sources"], ["a", "b"])
        self.assertEqual(
            got["edges"], [{"from": "a", "to": "c"}, {"from": "b", "to": "c"}]
        )
        self.assertEqual(got["unknown"], [{"from": "a", "specifier": "x"}])


if __name__ == "__main__":
    unittest.main()
