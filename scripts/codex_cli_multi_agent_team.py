#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import difflib
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import textwrap
import threading
from dataclasses import dataclass
from pathlib import Path


DEFAULT_WORKSPACE = Path(__file__).resolve().parents[1]
DEFAULT_RUNS_DIR = DEFAULT_WORKSPACE / ".codex-team-runs"
DEFAULT_HISTORY_LIMIT = 6
DEFAULT_HISTORY_MAX_CHARS = 12000


@dataclass(frozen=True)
class WorkerSpec:
    name: str
    sandbox: str
    full_auto: bool


PLANNER = WorkerSpec(name="planner", sandbox="read-only", full_auto=False)
IMPLEMENTER = WorkerSpec(name="implementer", sandbox="workspace-write", full_auto=True)
TESTER = WorkerSpec(name="tester", sandbox="workspace-write", full_auto=True)
REVIEWER = WorkerSpec(name="reviewer", sandbox="read-only", full_auto=False)

STATUS_PATTERN = re.compile(r"^STATUS:\s*(PASS|FAIL)\s*$", re.IGNORECASE | re.MULTILINE)
TASK_PATTERN = re.compile(r"Task:\s*(.*?)\n\s*Output format rules:", re.DOTALL)
SECTION_PATTERN = re.compile(
    r"^\s*(?:#+\s*)?(?:\*\*)?(Summary|Details|Verification)(?:\*\*)?\s*$",
    re.IGNORECASE | re.MULTILINE,
)
REL_PATH_PATTERN = re.compile(r"(?:[A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+")
ABS_PATH_PATTERN = re.compile(r"/[A-Za-z0-9_./-]+")


@dataclass(frozen=True)
class HistoryEntry:
    run_id: str
    run_dir: Path
    task: str
    overall_status: str
    current_worker: str
    step: str
    message: str
    updated_at: str
    score: float
    planner_status: str
    implementer_status: str
    tester_status: str
    reviewer_status: str
    planner_summary: str
    implementer_summary: str
    tester_summary: str
    reviewer_summary: str
    files: tuple[str, ...]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run a local multi-agent software team on top of codex exec."
    )
    parser.add_argument("--task", required=True, help="Task to assign to the team.")
    parser.add_argument(
        "--workspace",
        default=str(DEFAULT_WORKSPACE),
        help="Workspace directory for the codex workers.",
    )
    parser.add_argument(
        "--runs-dir",
        default=str(DEFAULT_RUNS_DIR),
        help="Directory where per-run logs and outputs are stored.",
    )
    parser.add_argument(
        "--model",
        default=os.getenv("CODEX_TEAM_MODEL", ""),
        help="Optional model override passed to codex exec.",
    )
    parser.add_argument(
        "--search",
        action="store_true",
        help="Enable codex web search for workers.",
    )
    parser.add_argument(
        "--skip-git-repo-check",
        action="store_true",
        help="Skip codex git repo checks.",
    )
    parser.add_argument(
        "--max-fix-rounds",
        type=int,
        default=1,
        help="How many fix rounds to allow after tester/reviewer failures.",
    )
    parser.add_argument(
        "--codex-bin",
        default=os.getenv("CODEX_BIN", "codex"),
        help="Path to the codex executable.",
    )
    parser.add_argument(
        "--stream",
        action="store_true",
        help="Stream worker stdout/stderr to the current terminal while also writing log files.",
    )
    parser.add_argument(
        "--dangerously-bypass-sandbox",
        action="store_true",
        help="Pass Codex's --dangerously-bypass-approvals-and-sandbox flag to every worker. Use only in a trusted environment.",
    )
    parser.add_argument(
        "--skip-planner",
        action="store_true",
        help="Skip the planner worker and start directly from implementer.",
    )
    parser.add_argument(
        "--worker-timeout-seconds",
        type=int,
        default=0,
        help="Optional timeout for each worker. Zero disables timeouts.",
    )
    parser.add_argument(
        "--history-limit",
        type=int,
        default=DEFAULT_HISTORY_LIMIT,
        help="How many prior runs to mine for prompt context. Zero disables history injection.",
    )
    parser.add_argument(
        "--history-max-chars",
        type=int,
        default=DEFAULT_HISTORY_MAX_CHARS,
        help="Maximum prompt characters reserved for summarized history context.",
    )
    return parser.parse_args()


def require_codex(binary: str) -> str:
    candidate = binary.strip()
    if candidate:
        if os.sep in candidate:
            path = Path(candidate).expanduser().resolve()
            if path.is_file() and os.access(path, os.X_OK):
                return str(path)
        else:
            resolved = shutil.which(candidate)
            if resolved:
                return resolved

    discovered = discover_codex_binaries()
    if discovered:
        return discovered[0]

    raise SystemExit(
        "codex executable not found. Set --codex-bin explicitly or install/sign in via the Codex-capable VS Code extension."
    )


def discover_codex_binaries() -> list[str]:
    home = Path.home()
    patterns = [
        str(home / ".vscode" / "extensions" / "openai.chatgpt-*" / "bin" / "*" / "codex"),
        str(home / ".vscode-insiders" / "extensions" / "openai.chatgpt-*" / "bin" / "*" / "codex"),
        str(home / ".cursor" / "extensions" / "openai.chatgpt-*" / "bin" / "*" / "codex"),
    ]

    matches: list[Path] = []
    for pattern in patterns:
        matches.extend(Path(path) for path in glob.glob(pattern))

    unique_paths: dict[str, Path] = {}
    for path in matches:
        try:
            resolved = path.expanduser().resolve()
        except FileNotFoundError:
            continue
        if resolved.is_file() and os.access(resolved, os.X_OK):
            unique_paths[str(resolved)] = resolved

    return [
        str(path)
        for path in sorted(
            unique_paths.values(),
            key=lambda item: (item.stat().st_mtime, str(item)),
            reverse=True,
        )
    ]


def read_optional_text(path: Path) -> str:
    if not path.exists():
        return ""
    return path.read_text(encoding="utf-8", errors="replace")


def parse_task_from_prompt(text: str) -> str:
    match = TASK_PATTERN.search(text)
    if not match:
        return ""
    return " ".join(line.strip() for line in match.group(1).strip().splitlines()).strip()


def extract_section(text: str, section_name: str) -> str:
    if not text.strip():
        return ""
    headings = list(SECTION_PATTERN.finditer(text))
    for index, heading in enumerate(headings):
        if heading.group(1).lower() != section_name.lower():
            continue
        start = heading.end()
        end = len(text)
        if index + 1 < len(headings):
            end = headings[index + 1].start()
        section = text[start:end].strip()
        return " ".join(section.split())
    return ""


def shorten(text: str, limit: int) -> str:
    text = " ".join(text.split())
    if len(text) <= limit:
        return text
    if limit <= 1:
        return text[:limit]
    return text[: limit - 1].rstrip() + "…"


def truncate_multiline(text: str, limit: int) -> str:
    text = text.strip()
    if len(text) <= limit:
        return text
    if limit <= 1:
        return text[:limit]
    return text[: limit - 1].rstrip() + "…"


def task_similarity(left: str, right: str) -> float:
    left = left.strip()
    right = right.strip()
    if not left or not right:
        return 0.0
    return difflib.SequenceMatcher(None, left.lower(), right.lower()).ratio()


def normalize_repo_path(candidate: str, workspace: Path) -> str | None:
    candidate = candidate.strip().strip("`'\"()[]{}<>,.;:")
    if not candidate:
        return None
    if candidate in {".", "./", "...", "./..."}:
        return None

    path = Path(candidate)
    if path.is_absolute():
        try:
            resolved = path.expanduser().resolve()
            relative = resolved.relative_to(workspace)
        except (FileNotFoundError, ValueError):
            return None
        normalized = relative.as_posix()
        if normalized == ".":
            return None
        return normalized

    relative = Path(candidate)
    if relative.as_posix().startswith("./"):
        relative = Path(relative.as_posix()[2:])
    try:
        resolved = (workspace / relative).resolve()
        resolved.relative_to(workspace)
    except (FileNotFoundError, ValueError):
        return None
    if resolved.exists():
        normalized = relative.as_posix()
        if normalized == ".":
            return None
        return normalized
    return None


def extract_repo_paths(text: str, workspace: Path) -> tuple[str, ...]:
    candidates: list[str] = []
    for pattern in (ABS_PATH_PATTERN, REL_PATH_PATTERN):
        for match in pattern.finditer(text):
            normalized = normalize_repo_path(match.group(0), workspace)
            if normalized:
                candidates.append(normalized)

    unique: list[str] = []
    seen: set[str] = set()
    for candidate in candidates:
        if candidate in seen:
            continue
        seen.add(candidate)
        unique.append(candidate)
    return tuple(unique)


def load_status_payload(run_dir: Path) -> dict[str, str]:
    status_path = run_dir / "status.json"
    if not status_path.exists():
        return {}
    try:
        payload = json.loads(status_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}
    return {
        "overall_status": str(payload.get("overall_status", "")),
        "current_worker": str(payload.get("current_worker", "")),
        "step": str(payload.get("step", "")),
        "message": str(payload.get("message", "")),
        "updated_at": str(payload.get("updated_at", "")),
    }


def load_history_entry(run_dir: Path, workspace: Path, task: str) -> HistoryEntry | None:
    if not run_dir.is_dir():
        return None

    prompt_text = ""
    for prompt_name in (
        "implementer.prompt.txt",
        "planner.prompt.txt",
        "tester.prompt.txt",
        "reviewer.prompt.txt",
    ):
        prompt_text = read_optional_text(run_dir / prompt_name)
        if prompt_text.strip():
            break

    run_task = parse_task_from_prompt(prompt_text)
    if not run_task:
        manifest_text = read_optional_text(run_dir / "run_manifest.json")
        if manifest_text:
            try:
                run_task = str(json.loads(manifest_text).get("task", "")).strip()
            except json.JSONDecodeError:
                run_task = ""

    status_payload = load_status_payload(run_dir)
    planner_text = read_optional_text(run_dir / "planner.final.txt")
    implementer_text = read_optional_text(run_dir / "implementer.final.txt")
    tester_text = read_optional_text(run_dir / "tester.final.txt")
    reviewer_text = read_optional_text(run_dir / "reviewer.final.txt")

    files = extract_repo_paths(
        "\n".join([planner_text, implementer_text, tester_text, reviewer_text]),
        workspace,
    )

    if not run_task and not any((planner_text, implementer_text, tester_text, reviewer_text)):
        return None

    return HistoryEntry(
        run_id=run_dir.name,
        run_dir=run_dir,
        task=run_task,
        overall_status=status_payload.get("overall_status", ""),
        current_worker=status_payload.get("current_worker", ""),
        step=status_payload.get("step", ""),
        message=status_payload.get("message", ""),
        updated_at=status_payload.get("updated_at", ""),
        score=task_similarity(task, run_task),
        planner_status=extract_status(planner_text) if planner_text else "",
        implementer_status=extract_status(implementer_text) if implementer_text else "",
        tester_status=extract_status(tester_text) if tester_text else "",
        reviewer_status=extract_status(reviewer_text) if reviewer_text else "",
        planner_summary=extract_section(planner_text, "Summary"),
        implementer_summary=extract_section(implementer_text, "Summary"),
        tester_summary=extract_section(tester_text, "Summary"),
        reviewer_summary=extract_section(reviewer_text, "Summary"),
        files=files,
    )


def select_history_entries(
    *,
    task: str,
    workspace: Path,
    runs_dir: Path,
    current_run_dir: Path,
    history_limit: int,
) -> list[HistoryEntry]:
    if history_limit <= 0 or not runs_dir.exists():
        return []

    entries: list[HistoryEntry] = []
    for child in runs_dir.iterdir():
        if child.name == "latest" or child == current_run_dir:
            continue
        entry = load_history_entry(child, workspace, task)
        if entry is None:
            continue
        entries.append(entry)

    entries.sort(
        key=lambda item: (item.score, item.updated_at, item.run_id),
        reverse=True,
    )

    selected: list[HistoryEntry] = []
    for entry in entries:
        if len(selected) >= history_limit:
            break
        if entry.score <= 0 and entry.overall_status != "failed":
            continue
        if entry.score < 0.05 and entry.overall_status not in {"failed", "completed"}:
            continue
        selected.append(entry)

    if selected:
        return selected

    return entries[:history_limit]


def format_history_context(entries: list[HistoryEntry], max_chars: int) -> str:
    if not entries or max_chars <= 0:
        return ""

    lines = [
        "Historical context from prior local multi-agent runs:",
        "",
    ]
    for entry in entries:
        headline = (
            f"- Run {entry.run_id} | similarity={entry.score:.2f} | "
            f"overall={entry.overall_status or 'unknown'} | "
            f"worker={entry.current_worker or 'n/a'}"
        )
        lines.append(headline)
        if entry.task:
            lines.append(f"  Task: {shorten(entry.task, 220)}")
        if entry.message:
            lines.append(f"  Status: {shorten(entry.message, 220)}")
        if entry.files:
            lines.append(
                f"  Files: {', '.join(entry.files[:6])}"
                + (" ..." if len(entry.files) > 6 else "")
            )

        summaries = []
        if entry.planner_summary:
            summaries.append(f"planner={shorten(entry.planner_summary, 180)}")
        if entry.implementer_summary:
            summaries.append(f"implementer={shorten(entry.implementer_summary, 180)}")
        if entry.tester_summary:
            summaries.append(f"tester={shorten(entry.tester_summary, 180)}")
        if entry.reviewer_summary:
            summaries.append(f"reviewer={shorten(entry.reviewer_summary, 180)}")
        if summaries:
            lines.append(f"  Notes: {' | '.join(summaries)}")
        lines.append("")

    context = "\n".join(lines).strip()
    return truncate_multiline(context, max_chars)


def write_history_artifacts(run_dir: Path, entries: list[HistoryEntry], context: str) -> None:
    history_payload = [
        {
            "run_id": entry.run_id,
            "task": entry.task,
            "overall_status": entry.overall_status,
            "current_worker": entry.current_worker,
            "step": entry.step,
            "message": entry.message,
            "updated_at": entry.updated_at,
            "score": round(entry.score, 4),
            "files": list(entry.files),
            "planner_status": entry.planner_status,
            "implementer_status": entry.implementer_status,
            "tester_status": entry.tester_status,
            "reviewer_status": entry.reviewer_status,
            "planner_summary": entry.planner_summary,
            "implementer_summary": entry.implementer_summary,
            "tester_summary": entry.tester_summary,
            "reviewer_summary": entry.reviewer_summary,
        }
        for entry in entries
    ]
    (run_dir / "history_runs.json").write_text(
        json.dumps(history_payload, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    (run_dir / "history_context.txt").write_text(context + ("\n" if context else ""), encoding="utf-8")


def write_run_manifest(
    run_dir: Path,
    *,
    task: str,
    workspace: Path,
    selected_history_entries: list[HistoryEntry],
    worker_statuses: dict[str, str] | None = None,
) -> None:
    payload = {
        "run_id": run_dir.name,
        "task": task,
        "workspace": str(workspace),
        "selected_history_runs": [entry.run_id for entry in selected_history_entries],
        "history_context_file": str(run_dir / "history_context.txt"),
        "worker_statuses": worker_statuses or {},
        "updated_at": dt.datetime.now(dt.UTC).isoformat(),
    }
    (run_dir / "run_manifest.json").write_text(
        json.dumps(payload, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )


def top_level_entries(workspace: Path) -> str:
    entries = []
    for child in sorted(workspace.iterdir(), key=lambda item: item.name.lower()):
        if child.name.startswith("."):
            continue
        entries.append(child.name + ("/" if child.is_dir() else ""))
    return ", ".join(entries)


def worker_prompt(
    worker: WorkerSpec,
    *,
    task: str,
    workspace: Path,
    history_context: str,
    extra_context: str,
) -> str:
    shared = f"""
    Workspace: {workspace}
    Top-level entries: {top_level_entries(workspace)}

    Task:
    {task}

    Output format rules:
    - First line must be exactly `STATUS: PASS` or `STATUS: FAIL`.
    - Then provide short sections named `Summary`, `Details`, and `Verification`.
    - Be concrete and repository-specific.
    """

    if worker is PLANNER:
        role = """
        You are the planning agent.
        Inspect the repository and create an implementation plan.
        Do not edit files.
        The `Details` section must list the files or directories likely involved and the execution order.
        """
    elif worker is IMPLEMENTER:
        role = """
        You are the implementation agent.
        Make the code changes needed for the task.
        Run focused checks for the changed area.
        The `Details` section must list the files you changed.
        If blocked, set `STATUS: FAIL` and explain the blocker precisely.
        """
    elif worker is TESTER:
        role = """
        You are the verification agent.
        Do not make code changes unless a command strictly requires generated files during verification.
        Run the smallest sufficient test/build/smoke commands.
        Set `STATUS: FAIL` if verification fails or coverage is clearly missing.
        """
    else:
        role = """
        You are the review agent.
        Do not edit files.
        Focus on bugs, regressions, risky assumptions, and missing tests.
        Set `STATUS: FAIL` if you found a blocking issue.
        """

    parts = [shared, role]
    if history_context.strip():
        parts.append(history_context)
    if extra_context.strip():
        parts.append(extra_context)
    return textwrap.dedent("\n\n".join(parts)).strip() + "\n"


def write_run_status(
    run_dir: Path,
    *,
    overall_status: str,
    current_worker: str,
    step: str,
    message: str,
) -> None:
    payload = {
        "overall_status": overall_status,
        "current_worker": current_worker,
        "step": step,
        "message": message,
        "updated_at": dt.datetime.now(dt.UTC).isoformat(),
    }
    (run_dir / "status.json").write_text(
        json.dumps(payload, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    (run_dir / "current_phase.txt").write_text(
        f"{overall_status} | {current_worker} | {step}\n{message}\n",
        encoding="utf-8",
    )


def ensure_latest_symlink(runs_dir: Path, run_dir: Path) -> None:
    latest = runs_dir / "latest"
    try:
        if latest.exists() or latest.is_symlink():
            latest.unlink()
        latest.symlink_to(run_dir.name)
    except OSError:
        (runs_dir / "latest.txt").write_text(str(run_dir) + "\n", encoding="utf-8")


def stream_pipe(pipe, log_handle, console_handle, prefix: str) -> None:
    try:
        for line in pipe:
            log_handle.write(line)
            log_handle.flush()
            if console_handle is not None:
                console_handle.write(f"[{prefix}] {line}")
                console_handle.flush()
    finally:
        pipe.close()


def run_worker(
    *,
    codex_bin: str,
    worker: WorkerSpec,
    workspace: Path,
    run_dir: Path,
    task: str,
    model: str,
    enable_search: bool,
    skip_git_repo_check: bool,
    stream_output: bool,
    bypass_sandbox: bool,
    worker_timeout_seconds: int,
    history_context: str,
    extra_context: str,
) -> str:
    output_path = run_dir / f"{worker.name}.final.txt"
    stdout_path = run_dir / f"{worker.name}.stdout.log"
    stderr_path = run_dir / f"{worker.name}.stderr.log"
    prompt_path = run_dir / f"{worker.name}.prompt.txt"

    cmd = [
        codex_bin,
        "exec",
        "--cd",
        str(workspace),
        "--sandbox",
        worker.sandbox,
        "--ephemeral",
        "--color",
        "never",
        "--output-last-message",
        str(output_path),
    ]
    if bypass_sandbox:
        cmd.append("--dangerously-bypass-approvals-and-sandbox")
    elif worker.full_auto:
        cmd.append("--full-auto")
    if skip_git_repo_check:
        cmd.append("--skip-git-repo-check")
    if enable_search:
        cmd.append("--search")
    if model:
        cmd.extend(["--model", model])

    prompt = worker_prompt(
        worker,
        task=task,
        workspace=workspace,
        history_context=history_context,
        extra_context=extra_context,
    )
    prompt_path.write_text(prompt, encoding="utf-8")

    print(
        f"[team] starting {worker.name} | stdout={stdout_path.name} stderr={stderr_path.name}",
        file=sys.stderr,
    )
    write_run_status(
        run_dir,
        overall_status="running",
        current_worker=worker.name,
        step="started",
        message=f"Running {worker.name}. Tail: tail -f {stdout_path}",
    )

    with stdout_path.open("w", encoding="utf-8") as stdout_handle, stderr_path.open(
        "w", encoding="utf-8"
    ) as stderr_handle:
        timed_out = False
        if stream_output:
            process = subprocess.Popen(
                cmd,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            assert process.stdin is not None
            process.stdin.write(prompt)
            process.stdin.close()

            stdout_thread = threading.Thread(
                target=stream_pipe,
                args=(process.stdout, stdout_handle, sys.stdout, f"{worker.name}:stdout"),
                daemon=True,
            )
            stderr_thread = threading.Thread(
                target=stream_pipe,
                args=(process.stderr, stderr_handle, sys.stderr, f"{worker.name}:stderr"),
                daemon=True,
            )
            stdout_thread.start()
            stderr_thread.start()
            try:
                completed_returncode = process.wait(
                    timeout=worker_timeout_seconds if worker_timeout_seconds > 0 else None
                )
            except subprocess.TimeoutExpired:
                timed_out = True
                stderr_handle.write(
                    f"\nworker timed out after {worker_timeout_seconds}s\n"
                )
                stderr_handle.flush()
                process.kill()
                completed_returncode = process.wait()
            stdout_thread.join()
            stderr_thread.join()
        else:
            try:
                completed = subprocess.run(
                    cmd,
                    input=prompt,
                    text=True,
                    stdout=stdout_handle,
                    stderr=stderr_handle,
                    check=False,
                    timeout=worker_timeout_seconds if worker_timeout_seconds > 0 else None,
                )
                completed_returncode = completed.returncode
            except subprocess.TimeoutExpired:
                timed_out = True
                stderr_handle.write(
                    f"\nworker timed out after {worker_timeout_seconds}s\n"
                )
                stderr_handle.flush()
                completed_returncode = 124

    output_text = output_path.read_text(encoding="utf-8") if output_path.exists() else ""
    final_status = extract_status(output_text) if output_text.strip() else "FAIL"
    write_run_status(
        run_dir,
        overall_status="running",
        current_worker=worker.name,
        step="completed",
        message=f"{worker.name} finished with status {final_status}",
    )
    if timed_out:
        write_run_status(
            run_dir,
            overall_status="failed",
            current_worker=worker.name,
            step="failed",
            message=f"{worker.name} timed out after {worker_timeout_seconds}s",
        )
        raise RuntimeError(f"{worker.name} timed out after {worker_timeout_seconds}s")
    if completed_returncode != 0 and not output_text.strip():
        stderr_text = stderr_path.read_text(encoding="utf-8", errors="replace")
        write_run_status(
            run_dir,
            overall_status="failed",
            current_worker=worker.name,
            step="failed",
            message=f"{worker.name} exited with code {completed_returncode}",
        )
        if "Sandbox(LandlockRestrict)" in stderr_text:
            raise RuntimeError(
                f"{worker.name} failed because Codex sandbox initialization is blocked by the host environment.\n"
                "Re-run with --dangerously-bypass-sandbox if you trust this machine.\n\n"
                f"{stderr_text}"
            )
        raise RuntimeError(
            f"{worker.name} failed with exit code {completed_returncode}\n{stderr_text}"
        )
    return output_text.strip()


def extract_status(text: str) -> str:
    match = STATUS_PATTERN.search(text)
    if not match:
        return "FAIL"
    return match.group(1).upper()


def compose_fix_context(
    planner_output: str,
    implementer_output: str,
    tester_output: str,
    reviewer_output: str,
) -> str:
    return (
        "Existing context from the previous round:\n\n"
        f"[Planner]\n{planner_output}\n\n"
        f"[Implementer]\n{implementer_output}\n\n"
        f"[Tester]\n{tester_output}\n\n"
        f"[Reviewer]\n{reviewer_output}\n\n"
        "Address the failures above and update the implementation."
    )


def run_team(args: argparse.Namespace) -> tuple[Path, str]:
    workspace = Path(args.workspace).expanduser().resolve()
    if not workspace.is_dir():
        raise SystemExit(f"Workspace is not a directory: {workspace}")

    runs_dir = Path(args.runs_dir).expanduser().resolve()
    runs_dir.mkdir(parents=True, exist_ok=True)
    run_dir = runs_dir / dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    run_dir.mkdir(parents=True, exist_ok=True)
    ensure_latest_symlink(runs_dir, run_dir)

    codex_bin = require_codex(args.codex_bin)
    selected_history_entries = select_history_entries(
        task=args.task,
        workspace=workspace,
        runs_dir=runs_dir,
        current_run_dir=run_dir,
        history_limit=args.history_limit,
    )
    history_context = format_history_context(selected_history_entries, args.history_max_chars)
    write_history_artifacts(run_dir, selected_history_entries, history_context)
    write_run_manifest(
        run_dir,
        task=args.task,
        workspace=workspace,
        selected_history_entries=selected_history_entries,
    )
    print(f"Using codex binary: {codex_bin}", file=sys.stderr)
    print(f"Run directory: {run_dir}", file=sys.stderr)
    print(f"Watch status:  cat {run_dir / 'current_phase.txt'}", file=sys.stderr)
    print(f"Latest link:   {runs_dir / 'latest'}", file=sys.stderr)
    print(
        f"History context: {len(selected_history_entries)} prior run(s) -> {run_dir / 'history_context.txt'}",
        file=sys.stderr,
    )
    if args.dangerously_bypass_sandbox:
        print("Codex sandbox: BYPASSED", file=sys.stderr)
    skip_git_repo_check = args.skip_git_repo_check or not (workspace / ".git").exists()
    write_run_status(
        run_dir,
        overall_status="running",
        current_worker="manager",
        step="initialized",
        message="Multi-agent run initialized",
    )

    if args.skip_planner:
        planner_output = textwrap.dedent(
            """
            STATUS: PASS

            Summary
            Planner skipped by operator request.

            Details
            Proceed directly to implementation using the provided task.

            Verification
            Planner was intentionally skipped.
            """
        ).strip()
        (run_dir / "planner.final.txt").write_text(planner_output + "\n", encoding="utf-8")
    else:
        planner_output = run_worker(
            codex_bin=codex_bin,
            worker=PLANNER,
            workspace=workspace,
            run_dir=run_dir,
            task=args.task,
            model=args.model,
            enable_search=args.search,
            skip_git_repo_check=skip_git_repo_check,
            stream_output=args.stream,
            bypass_sandbox=args.dangerously_bypass_sandbox,
            worker_timeout_seconds=args.worker_timeout_seconds,
            history_context=history_context,
            extra_context="Produce the plan only.",
        )

    implementer_output = run_worker(
        codex_bin=codex_bin,
        worker=IMPLEMENTER,
        workspace=workspace,
        run_dir=run_dir,
        task=args.task,
        model=args.model,
        enable_search=args.search,
        skip_git_repo_check=skip_git_repo_check,
        stream_output=args.stream,
        bypass_sandbox=args.dangerously_bypass_sandbox,
        worker_timeout_seconds=args.worker_timeout_seconds,
        history_context=history_context,
        extra_context=f"Plan from planner:\n\n{planner_output}",
    )

    tester_output = run_worker(
        codex_bin=codex_bin,
        worker=TESTER,
        workspace=workspace,
        run_dir=run_dir,
        task=args.task,
        model=args.model,
        enable_search=args.search,
        skip_git_repo_check=skip_git_repo_check,
        stream_output=args.stream,
        bypass_sandbox=args.dangerously_bypass_sandbox,
        worker_timeout_seconds=args.worker_timeout_seconds,
        history_context=history_context,
        extra_context=(
            f"Planner output:\n\n{planner_output}\n\n"
            f"Implementer output:\n\n{implementer_output}"
        ),
    )

    reviewer_output = run_worker(
        codex_bin=codex_bin,
        worker=REVIEWER,
        workspace=workspace,
        run_dir=run_dir,
        task=args.task,
        model=args.model,
        enable_search=args.search,
        skip_git_repo_check=skip_git_repo_check,
        stream_output=args.stream,
        bypass_sandbox=args.dangerously_bypass_sandbox,
        worker_timeout_seconds=args.worker_timeout_seconds,
        history_context=history_context,
        extra_context=(
            f"Planner output:\n\n{planner_output}\n\n"
            f"Implementer output:\n\n{implementer_output}\n\n"
            f"Tester output:\n\n{tester_output}"
        ),
    )

    for round_index in range(args.max_fix_rounds):
        tester_status = extract_status(tester_output)
        reviewer_status = extract_status(reviewer_output)
        if tester_status == "PASS" and reviewer_status == "PASS":
            break

        implementer_output = run_worker(
            codex_bin=codex_bin,
            worker=IMPLEMENTER,
            workspace=workspace,
            run_dir=run_dir,
            task=args.task,
            model=args.model,
            enable_search=args.search,
            skip_git_repo_check=skip_git_repo_check,
            stream_output=args.stream,
            bypass_sandbox=args.dangerously_bypass_sandbox,
            worker_timeout_seconds=args.worker_timeout_seconds,
            history_context=history_context,
            extra_context=compose_fix_context(
                planner_output,
                implementer_output,
                tester_output,
                reviewer_output,
            )
            + f"\n\nThis is fix round {round_index + 1}.",
        )

        tester_output = run_worker(
            codex_bin=codex_bin,
            worker=TESTER,
            workspace=workspace,
            run_dir=run_dir,
            task=args.task,
            model=args.model,
            enable_search=args.search,
            skip_git_repo_check=skip_git_repo_check,
            stream_output=args.stream,
            bypass_sandbox=args.dangerously_bypass_sandbox,
            worker_timeout_seconds=args.worker_timeout_seconds,
            history_context=history_context,
            extra_context=(
                f"Planner output:\n\n{planner_output}\n\n"
                f"Implementer output after fix round {round_index + 1}:\n\n{implementer_output}"
            ),
        )

        reviewer_output = run_worker(
            codex_bin=codex_bin,
            worker=REVIEWER,
            workspace=workspace,
            run_dir=run_dir,
            task=args.task,
            model=args.model,
            enable_search=args.search,
            skip_git_repo_check=skip_git_repo_check,
            stream_output=args.stream,
            bypass_sandbox=args.dangerously_bypass_sandbox,
            worker_timeout_seconds=args.worker_timeout_seconds,
            history_context=history_context,
            extra_context=(
                f"Planner output:\n\n{planner_output}\n\n"
                f"Implementer output after fix round {round_index + 1}:\n\n{implementer_output}\n\n"
                f"Tester output after fix round {round_index + 1}:\n\n{tester_output}"
            ),
        )

    final_summary = textwrap.dedent(
        f"""
        Run directory: {run_dir}

        [Planner]
        {planner_output}

        [Implementer]
        {implementer_output}

        [Tester]
        {tester_output}

        [Reviewer]
        {reviewer_output}
        """
    ).strip()

    (run_dir / "summary.txt").write_text(final_summary + "\n", encoding="utf-8")
    write_run_manifest(
        run_dir,
        task=args.task,
        workspace=workspace,
        selected_history_entries=selected_history_entries,
        worker_statuses={
            "planner": extract_status(planner_output),
            "implementer": extract_status(implementer_output),
            "tester": extract_status(tester_output),
            "reviewer": extract_status(reviewer_output),
        },
    )
    write_run_status(
        run_dir,
        overall_status="completed",
        current_worker="manager",
        step="finished",
        message="Multi-agent run finished",
    )
    return run_dir, final_summary


def main() -> int:
    args = parse_args()
    try:
        run_dir, summary = run_team(args)
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print(summary)
    print(f"\nArtifacts saved under: {run_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
