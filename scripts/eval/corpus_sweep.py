#!/usr/bin/env python3
"""Run archfit deterministic corpus sweeps with temp configs.

- Uses the current branch binary from this repo.
- Runs from the archfit repo root so local .env loading is consistent.
- Never edits target-repo configs; it copies or initializes a temp config per repo.
- Treats config-update failures as findings, not silent skips.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKSPACE_ROOT = Path.home() / "workspace"
DEFAULT_ARCHFIT = REPO_ROOT / ".bin" / "archfit"
DEFAULT_OUTPUT_DIR = Path("/tmp/archfit-corpus-eval")
DEFAULT_SUMMARY_FILE = Path("/tmp/archfit-corpus-results.json")
DEFAULT_AI_REPOS = {"spotinfo", "ccgram", "herdr", "storybook"}


@dataclass(frozen=True)
class RepoSpec:
    label: str
    root: Path
    lang: str
    has_config: bool = True


CORPUS: dict[str, RepoSpec] = {
    "spotinfo": RepoSpec("spotinfo", WORKSPACE_ROOT / "spotinfo", "go"),
    "pumba": RepoSpec("pumba", WORKSPACE_ROOT / "pumba", "go"),
    "omni/scheduled-tasks": RepoSpec(
        "omni/scheduled-tasks",
        WORKSPACE_ROOT / "omni/server/services/scheduled-tasks",
        "go",
        has_config=False,
    ),
    "prometheus": RepoSpec("prometheus", WORKSPACE_ROOT / "prometheus", "go"),
    "ccgram": RepoSpec("ccgram", WORKSPACE_ROOT / "ccgram", "python"),
    "prefect": RepoSpec("prefect", WORKSPACE_ROOT / "prefect", "python"),
    "storybook": RepoSpec("storybook", WORKSPACE_ROOT / "storybook", "ts"),
    "yazi": RepoSpec("yazi", WORKSPACE_ROOT / "yazi", "rust"),
    "herdr": RepoSpec("herdr", WORKSPACE_ROOT / "herdr", "rust"),
    "ruff": RepoSpec("ruff", WORKSPACE_ROOT / "ruff", "rust"),
    "tokio": RepoSpec("tokio", WORKSPACE_ROOT / "tokio", "rust"),
}


def parse_csv_set(raw: str | None) -> set[str]:
    if not raw:
        return set()
    return {item.strip() for item in raw.split(",") if item.strip()}


def sanitize(label: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]+", "_", label)


def run(
    cmd: list[str],
    *,
    cwd: Path,
    stdout_path: Path | None = None,
    stderr_path: Path | None = None,
) -> subprocess.CompletedProcess[str]:
    proc = subprocess.run(
        cmd,
        cwd=str(cwd),
        text=True,
        capture_output=True,
        env=os.environ.copy(),
    )
    if stdout_path is not None:
        stdout_path.write_text(proc.stdout)
    if stderr_path is not None:
        stderr_path.write_text(proc.stderr)
    return proc


def ensure_ai_block(cfg_path: Path) -> None:
    text = cfg_path.read_text() if cfg_path.exists() else ""
    if re.search(r"(?m)^ai:\s*$", text):
        return
    suffix = "\nai:\n  provider: anthropic\n  model: claude-opus-4-8\n"
    cfg_path.write_text(text.rstrip() + suffix)


def summarize_json(path: Path) -> dict[str, Any]:
    data = json.loads(path.read_text())
    findings = data.get("findings", []) or []
    coverage = data.get("tool_coverage", []) or []
    coverage_counts: dict[str, int] = {}
    for row in coverage:
        status = row.get("status", "unknown")
        coverage_counts[status] = coverage_counts.get(status, 0) + 1
    return {
        "verdict": data.get("verdict"),
        "summary": data.get("summary"),
        "score_overall": data.get("score", {}).get("overall"),
        "score_band": data.get("score", {}).get("overall_band"),
        "score_version": data.get("score_version"),
        "findings_total": len(findings),
        "coverage_status_counts": coverage_counts,
        "findings_sample": [
            {
                "id": f.get("id"),
                "kind": f.get("kind"),
                "rule_id": f.get("rule_id"),
                "status": f.get("status"),
                "severity": f.get("severity"),
                "from": ((f.get("edge") or {}).get("from") or {}).get("module")
                or ((f.get("edge") or {}).get("from") or {}).get("path"),
                "to": ((f.get("edge") or {}).get("to") or {}).get("module")
                or ((f.get("edge") or {}).get("to") or {}).get("path"),
            }
            for f in findings[:5]
        ],
    }


def ai_excerpt(markdown_path: Path) -> str:
    if not markdown_path.exists():
        return ""
    lines = markdown_path.read_text().splitlines()
    if len(lines) <= 20:
        return "\n".join(lines).strip()
    return "\n".join(lines[-20:]).strip()


def compare_json(a_path: Path, b_path: Path) -> bool | None:
    if not a_path.exists() or not b_path.exists():
        return None
    try:
        return json.loads(a_path.read_text()) == json.loads(b_path.read_text())
    except json.JSONDecodeError:
        return None


def prepare_temp_config(
    spec: RepoSpec, tmp_dir: Path, archfit_bin: Path, *, want_ai: bool
) -> tuple[Path | None, dict[str, Any]]:
    cfg = tmp_dir / "archfit.yaml"
    meta: dict[str, Any] = {}
    existing = spec.root / ".archfit.yaml"
    if existing.exists():
        shutil.copy2(existing, cfg)
        meta["config_source"] = "copied-existing"
    else:
        init = run(
            [
                str(archfit_bin),
                "config",
                "init",
                "--root",
                str(spec.root),
                "--output",
                str(cfg),
            ],
            cwd=REPO_ROOT,
            stdout_path=tmp_dir / "config-init.stdout",
            stderr_path=tmp_dir / "config-init.stderr",
        )
        meta["config_init_exit"] = init.returncode
        meta["config_source"] = "init"
        if init.returncode != 0 or not cfg.exists():
            meta["config_error"] = "config init failed"
            return None, meta
    if want_ai:
        ensure_ai_block(cfg)
        meta["config_source"] += "+ai"
    return cfg, meta


def process_repo(
    spec: RepoSpec,
    archfit_bin: Path,
    output_dir: Path,
    want_ai: bool,
    repeat: bool,
    skip_update: bool,
) -> dict[str, Any]:
    result: dict[str, Any] = {
        "label": spec.label,
        "root": str(spec.root),
        "lang": spec.lang,
        "artifacts": {},
    }
    if not spec.root.exists():
        result["error"] = "repo missing"
        return result

    tmp_dir = output_dir / sanitize(spec.label)
    if tmp_dir.exists():
        shutil.rmtree(tmp_dir)
    tmp_dir.mkdir(parents=True, exist_ok=True)

    cfg, prep_meta = prepare_temp_config(spec, tmp_dir, archfit_bin, want_ai=want_ai)
    result.update(prep_meta)
    if cfg is None:
        return result
    result["config_path"] = str(cfg)

    if not skip_update:
        update = run(
            [
                str(archfit_bin),
                "config",
                "update",
                "--apply",
                "-c",
                str(cfg),
                "-r",
                str(spec.root),
            ],
            cwd=REPO_ROOT,
            stdout_path=tmp_dir / "config-update.stdout",
            stderr_path=tmp_dir / "config-update.stderr",
        )
        result["config_update_exit"] = update.returncode
        result["artifacts"]["config_update_stdout"] = str(
            tmp_dir / "config-update.stdout"
        )
        result["artifacts"]["config_update_stderr"] = str(
            tmp_dir / "config-update.stderr"
        )
        if update.returncode != 0:
            result["config_update_error_tail"] = (
                (tmp_dir / "config-update.stderr").read_text().splitlines()[-8:]
            )
    else:
        result["config_update_exit"] = None

    def collect_json_run(kind: str, cmd: list[str]) -> None:
        stdout_path = tmp_dir / f"{kind}.json"
        stderr_path = tmp_dir / f"{kind}.stderr"
        proc = run(cmd, cwd=REPO_ROOT, stdout_path=stdout_path, stderr_path=stderr_path)
        result[f"{kind}_exit"] = proc.returncode
        result["artifacts"][f"{kind}_json"] = str(stdout_path)
        result["artifacts"][f"{kind}_stderr"] = str(stderr_path)
        if stdout_path.exists() and stdout_path.stat().st_size > 0:
            try:
                result[kind] = summarize_json(stdout_path)
            except Exception as exc:  # noqa: BLE001
                result[kind] = {"error": f"failed to parse json: {exc}"}
        else:
            result[kind] = {"error": "missing json output"}
            if stderr_path.exists():
                result[f"{kind}_stderr_tail"] = stderr_path.read_text().splitlines()[
                    -8:
                ]

    collect_json_run(
        "check",
        [str(archfit_bin), "check", "--json", "-c", str(cfg), "--root", str(spec.root)],
    )
    collect_json_run(
        "analyze",
        [
            str(archfit_bin),
            "analyze",
            "--json",
            "-c",
            str(cfg),
            "--root",
            str(spec.root),
        ],
    )

    if want_ai:
        ai_md = tmp_dir / "ai-summary.md"
        ai_err = tmp_dir / "ai-summary.stderr"
        ai = run(
            [
                str(archfit_bin),
                "analyze",
                "--ai-summary",
                "--markdown",
                "-c",
                str(cfg),
                "--root",
                str(spec.root),
            ],
            cwd=REPO_ROOT,
            stdout_path=ai_md,
            stderr_path=ai_err,
        )
        result["ai_exit"] = ai.returncode
        result["ai_excerpt"] = ai_excerpt(ai_md)
        result["artifacts"]["ai_markdown"] = str(ai_md)
        result["artifacts"]["ai_stderr"] = str(ai_err)
        if ai.returncode != 0:
            result["ai_stderr_tail"] = ai_err.read_text().splitlines()[-8:]

    if repeat:
        analyze_repeat = tmp_dir / "analyze-repeat.json"
        analyze_repeat_err = tmp_dir / "analyze-repeat.stderr"
        repeat_proc = run(
            [
                str(archfit_bin),
                "analyze",
                "--json",
                "-c",
                str(cfg),
                "--root",
                str(spec.root),
            ],
            cwd=REPO_ROOT,
            stdout_path=analyze_repeat,
            stderr_path=analyze_repeat_err,
        )
        result["repeat_analyze_exit"] = repeat_proc.returncode
        result["artifacts"]["repeat_analyze_json"] = str(analyze_repeat)
        result["artifacts"]["repeat_analyze_stderr"] = str(analyze_repeat_err)
        result["repeat_analyze_same_json"] = compare_json(
            tmp_dir / "analyze.json", analyze_repeat
        )

    return result


def print_progress(payload: dict[str, Any]) -> None:
    print(json.dumps(payload, sort_keys=True), flush=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--archfit",
        default=str(DEFAULT_ARCHFIT),
        help="Path to the archfit binary (default: ./.bin/archfit)",
    )
    parser.add_argument(
        "--repos",
        default=",".join(CORPUS.keys()),
        help="Comma-separated repo labels from the built-in corpus list",
    )
    parser.add_argument(
        "--ai-repos",
        default=",".join(sorted(DEFAULT_AI_REPOS)),
        help="Comma-separated repo labels to run analyze --ai-summary on",
    )
    parser.add_argument(
        "--repeat-repos",
        default="",
        help="Comma-separated repo labels to rerun analyze --json on for determinism checks",
    )
    parser.add_argument(
        "--output-dir",
        default=str(DEFAULT_OUTPUT_DIR),
        help="Artifact output directory",
    )
    parser.add_argument(
        "--summary-file",
        default=str(DEFAULT_SUMMARY_FILE),
        help="Summary JSON output file",
    )
    parser.add_argument(
        "--max-workers", type=int, default=4, help="Max concurrent repos"
    )
    parser.add_argument(
        "--skip-update",
        action="store_true",
        help="Skip config update --apply on temp configs",
    )
    args = parser.parse_args()

    archfit_bin = Path(args.archfit).resolve()
    if not archfit_bin.exists():
        print(f"error: archfit binary not found at {archfit_bin}", file=sys.stderr)
        return 1

    selected = parse_csv_set(args.repos)
    ai_repos = parse_csv_set(args.ai_repos)
    repeat_repos = parse_csv_set(args.repeat_repos)
    unknown = sorted(selected - CORPUS.keys())
    if unknown:
        print(f"error: unknown repo label(s): {', '.join(unknown)}", file=sys.stderr)
        return 1

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    summary_file = Path(args.summary_file)

    specs = [CORPUS[label] for label in CORPUS.keys() if label in selected]
    results: list[dict[str, Any]] = []

    with ThreadPoolExecutor(max_workers=max(args.max_workers, 1)) as pool:
        futures = {
            pool.submit(
                process_repo,
                spec,
                archfit_bin,
                output_dir,
                spec.label in ai_repos,
                spec.label in repeat_repos,
                args.skip_update,
            ): spec
            for spec in specs
        }
        for future in as_completed(futures):
            spec = futures[future]
            try:
                result = future.result()
            except Exception as exc:  # noqa: BLE001
                result = {
                    "label": spec.label,
                    "root": str(spec.root),
                    "lang": spec.lang,
                    "error": repr(exc),
                }
            results.append(result)
            check_summary = result.get("check")
            analyze_summary = result.get("analyze")
            print_progress(
                {
                    "label": result.get("label"),
                    "check_exit": result.get("check_exit"),
                    "check_verdict": check_summary.get("verdict")
                    if isinstance(check_summary, dict)
                    else None,
                    "analyze_exit": result.get("analyze_exit"),
                    "analyze_verdict": analyze_summary.get("verdict")
                    if isinstance(analyze_summary, dict)
                    else None,
                    "ai_exit": result.get("ai_exit"),
                    "config_update_exit": result.get("config_update_exit"),
                    "error": result.get("error"),
                }
            )

    results.sort(key=lambda item: item.get("label", ""))
    summary_file.write_text(json.dumps(results, indent=2, sort_keys=True))
    print_progress({"summary_file": str(summary_file), "repos": len(results)})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
