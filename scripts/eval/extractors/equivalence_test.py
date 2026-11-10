import unittest

from . import equivalence


def side(edges, unknown=(), sources=()):
    return {
        "edges": [{"from": f, "to": t} for f, t in edges],
        "unknown": [{"from": f, "specifier": s} for f, s in unknown],
        "sources": list(sources),
    }


class CompareTest(unittest.TestCase):
    module_of = {
        "a/x.ts": "a",
        "a/y.ts": "a",
        "b/z.ts": "b",
        "c/w.ts": "c",
        "loose.ts": "",
    }

    def test_file_and_module_levels(self):
        ref = side(
            [("a/x.ts", "b/z.ts"), ("a/x.ts", "a/y.ts"), ("a/y.ts", "c/w.ts")],
            sources=["a/x.ts", "a/y.ts"],
        )
        new = side(
            [("a/x.ts", "b/z.ts"), ("a/x.ts", "a/y.ts"), ("loose.ts", "b/z.ts")],
            unknown=[("a/y.ts", "./gone")],
            sources=["a/x.ts", "loose.ts"],
        )
        got = equivalence.compare(ref, new, self.module_of)
        self.assertEqual(got["files"]["both"], 2)
        self.assertEqual(got["files"]["only_reference"], [["a/y.ts", "c/w.ts"]])
        self.assertEqual(got["files"]["only_new"], [["loose.ts", "b/z.ts"]])
        self.assertEqual(got["sources"]["only_reference"], ["a/y.ts"])
        self.assertEqual(got["sources"]["only_new"], ["loose.ts"])
        modules = got["modules"]
        self.assertTrue(modules["mapped"])
        self.assertEqual(
            (modules["reference"], modules["new"], modules["both"]),
            (2, 2, 1),
            "a same-module edge is no pair",
        )
        self.assertEqual([p["pair"] for p in modules["only_reference"]], [["a", "c"]])
        self.assertEqual(
            [p["pair"] for p in modules["only_new"]], [[equivalence.NONE, "b"]]
        )
        self.assertEqual(got["unknown"]["new_share"], round(1 / 4, 4))

    def test_new_edges_the_reference_left_unresolved(self):
        ref = side([], unknown=[("a/x.ts", "pkg/sub"), ("a/y.ts", "./gone")])
        new = side([("a/x.ts", "b/z.ts"), ("a/y.ts", "c/w.ts")])
        new["resolutions"] = [
            {"from": "a/x.ts", "specifier": "pkg/sub", "to": "b/z.ts"},
            {"from": "a/y.ts", "specifier": "./other", "to": "c/w.ts"},
        ]
        got = equivalence.compare(ref, new, self.module_of)
        self.assertEqual(got["files"]["only_new_reference_unresolved"], 1)

    def test_without_a_mapping_there_are_no_pairs(self):
        got = equivalence.compare(
            side([("a/x.ts", "b/z.ts")]), side([("a/x.ts", "b/z.ts")]), {}
        )
        self.assertFalse(got["modules"]["mapped"])
        self.assertEqual(got["modules"]["reference"], 0)
        self.assertIn("no mapping", equivalence.summary("r", "typescript", got))


if __name__ == "__main__":
    unittest.main()
