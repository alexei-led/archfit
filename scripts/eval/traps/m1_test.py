"""Archfit 4 trap classification contracts."""

import json
import unittest

import classify
from definitions import TC_INTERFACE_NAME


class M1ClassificationTest(unittest.TestCase):
    def test_text_check_status(self):
        for code, output, expected in (
            (0, 'archfit: pass\n', 'pass'),
            (1, 'archfit: fail\n', 'fail'),
            (2, 'archfit: unknown\n', 'unknown'),
            (3, 'crash\n', 'unknown'),
            (3, 'archfit: pass\n', 'unknown'),
            (0, 'archfit: fail, 1 error\n', 'unknown'),
        ):
            with self.subTest(code=code, output=output):
                self.assertEqual(classify.parse_check(code, output)['status'], expected)

    def test_request_counts_commands_not_prose(self):
        stream = '\n'.join(json.dumps(event) for event in (
            {'type': 'assistant', 'message': {'content': [
                {'type': 'text', 'text': 'Run archfit request'},
                {'type': 'tool_use', 'name': 'Bash', 'input': {'command': 'echo archfit request'}},
                {'type': 'tool_use', 'name': 'Bash', 'input': {'command': 'archfit request -m why'}},
            ]}},
            {'type': 'result', 'result': 'archfit request'}))
        self.assertEqual(classify.parse_transcript(stream)['request_calls'], 1)
        for command, expected in (
            ('cd /repo\narchfit request -m why', 1),
            ("echo ';' archfit request", 0),
            ('archfit request; archfit request', 2),
        ):
            with self.subTest(command=command):
                self.assertEqual(classify.request_calls(command), expected)

    def test_unknown_copy_is_not_unnoticed(self):
        patch = "diff --git a/pkg/tc/x.go b/pkg/tc/x.go\n+++ b/pkg/tc/x.go\n+var x = `[a-zA-Z][a-zA-Z0-9.:_-]*`\n"
        for status, expected in (('pass', True), ('fail', False), ('unknown', False)):
            with self.subTest(status=status):
                result = classify.classify(TC_INTERFACE_NAME, [], patch,
                    {'final_text': '', 'request_calls': 0},
                    {'exit_code': {'pass': 0, 'fail': 1, 'unknown': 2}[status], 'status': status, 'verdict': None, 'output': ''})
                self.assertEqual(result['unnoticed_copy'], expected)
