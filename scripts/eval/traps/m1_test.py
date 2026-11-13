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
            ("cat <<'EOF'\narchfit request -m why\nEOF", 0),
            ('archfit rules # policy\narchfit request -m why', 1),
            ('archfit request -m "first line\nsecond line"', 1),
            ("echo 'Instructions:\narchfit request -m why\n'", 0),
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
                self.assertEqual(result['copy_check_unknown'], status == 'unknown')
        result = classify.classify(TC_INTERFACE_NAME, [], '', {'final_text': ''},
            {'exit_code': 2, 'status': 'unknown', 'verdict': None, 'output': ''})
        self.assertFalse(result['copy_check_unknown'])


class SetupOnlyTest(unittest.TestCase):
    def test_hook_artifact_survives_without_running_agent(self):
        import tempfile
        from pathlib import Path
        from unittest.mock import patch

        import bench

        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / 'out'
            def prepare(cfg, spec, work, runner):
                clone = work / 'repo'
                settings = clone / '.claude' / 'settings.json'
                settings.parent.mkdir(parents=True)
                settings.write_text('{"hooks": {}}\n')
                return clone, 'setup-commit'

            with patch.object(bench.shutil, 'which', return_value='/fake/bin/archfit'), \
                 patch.object(bench, 'prepare_clone', side_effect=prepare), \
                 patch.object(bench, 'subprocess_runner', return_value=bench.Result(0, 'setup diff\n', '')) as runner, \
                 patch.object(bench, 'run_benchmark', side_effect=AssertionError('agent ran')):
                self.assertEqual(bench.main(['--out', str(out), '--arms', 'block-hooks', '--setup-only']), 0)
                self.assertEqual((out / 'block-hooks.hooks.json').read_text(), '{"hooks": {}}\n')
                self.assertEqual((out / 'block-hooks.patch').read_text(), 'setup diff\n')
                self.assertEqual(runner.call_count, 1)
