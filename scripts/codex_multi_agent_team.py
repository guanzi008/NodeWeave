#!/usr/bin/env python3
from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path
from typing import Iterable

try:
    from agents import Agent, Runner
    from agents.extensions.experimental.codex import ThreadOptions, TurnOptions, codex_tool
except ImportError as exc:
    raise SystemExit(
        "Missing dependency. Install with: pip install -r scripts/requirements-codex-agents.txt"
    ) from exc


DEFAULT_WORKSPACE = Path(__file__).resolve().parents[1]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Orchestrate a Codex-based multi-agent development team with the OpenAI Agents SDK."
    )
    parser.add_argument(
        "--task",
        required=True,
        help="Development task for the agent team.",
    )
    parser.add_argument(
        "--workspace",
        default=str(DEFAULT_WORKSPACE),
        help="Workspace root for Codex to inspect and edit.",
    )
    parser.add_argument(
        "--manager-model",
        default=os.getenv("OPENAI_AGENTS_MANAGER_MODEL", "gpt-5.4"),
        help="Model for the top-level orchestration agent.",
    )
    parser.add_argument(
        "--codex-model",
        default=os.getenv("OPENAI_AGENTS_CODEX_MODEL", "gpt-5.2-codex"),
        help="Model used by Codex tool threads.",
    )
    parser.add_argument(
        "--reasoning-effort",
        default=os.getenv("OPENAI_AGENTS_REASONING_EFFORT", "low"),
        choices=["minimal", "low", "medium", "high"],
        help="Reasoning effort passed to Codex thread options.",
    )
    parser.add_argument(
        "--approval-policy",
        default=os.getenv("OPENAI_AGENTS_APPROVAL_POLICY", "never"),
        help="Codex approval policy.",
    )
    parser.add_argument(
        "--web-search-mode",
        default=os.getenv("OPENAI_AGENTS_WEB_SEARCH_MODE", "disabled"),
        help="Codex web search mode.",
    )
    parser.add_argument(
        "--network-access",
        action="store_true",
        help="Enable network access in Codex threads.",
    )
    parser.add_argument(
        "--idle-timeout-seconds",
        type=int,
        default=90,
        help="Idle timeout for each Codex turn.",
    )
    parser.add_argument(
        "--max-turns",
        type=int,
        default=18,
        help="Maximum top-level orchestrator turns.",
    )
    parser.add_argument(
        "--skip-git-repo-check",
        action="store_true",
        help="Skip Codex git repository check for workspaces that are not git repos.",
    )
    return parser.parse_args()


def require_api_key() -> None:
    if os.getenv("CODEX_API_KEY") or os.getenv("OPENAI_API_KEY"):
        return
    raise SystemExit("Set CODEX_API_KEY or OPENAI_API_KEY before running this script.")


def workspace_entries(workspace: Path) -> list[str]:
    entries = []
    for child in sorted(workspace.iterdir(), key=lambda item: item.name.lower()):
        if child.name.startswith("."):
            continue
        suffix = "/" if child.is_dir() else ""
        entries.append(child.name + suffix)
    return entries


def make_codex_tool(
    *,
    workspace: Path,
    sandbox_mode: str,
    codex_model: str,
    reasoning_effort: str,
    approval_policy: str,
    web_search_mode: str,
    network_access: bool,
    idle_timeout_seconds: int,
    skip_git_repo_check: bool,
):
    return codex_tool(
        sandbox_mode=sandbox_mode,
        working_directory=str(workspace),
        skip_git_repo_check=skip_git_repo_check,
        default_thread_options=ThreadOptions(
            model=codex_model,
            model_reasoning_effort=reasoning_effort,
            network_access_enabled=network_access,
            web_search_mode=web_search_mode,
            approval_policy=approval_policy,
        ),
        default_turn_options=TurnOptions(
            idle_timeout_seconds=idle_timeout_seconds,
        ),
        persist_session=True,
    )


def build_agents(args: argparse.Namespace, workspace: Path) -> Agent:
    skip_git_repo_check = args.skip_git_repo_check or not (workspace / ".git").exists()

    planner = Agent(
        name="Planner",
        model=args.manager_model,
        instructions=(
            "You are the technical lead for a coding task. "
            "Use the Codex tool to inspect the repository and produce an execution plan. "
            "Do not modify files. "
            "Return a concise plan with scope, relevant files, implementation order, and verification targets."
        ),
        tools=[
            make_codex_tool(
                workspace=workspace,
                sandbox_mode="read-only",
                codex_model=args.codex_model,
                reasoning_effort=args.reasoning_effort,
                approval_policy=args.approval_policy,
                web_search_mode=args.web_search_mode,
                network_access=args.network_access,
                idle_timeout_seconds=args.idle_timeout_seconds,
                skip_git_repo_check=skip_git_repo_check,
            )
        ],
    )

    implementer = Agent(
        name="Implementer",
        model=args.manager_model,
        instructions=(
            "You are the primary implementation engineer. "
            "Use the Codex tool to edit files, run focused verification, and keep changes scoped to the task. "
            "You should actually make the code changes when needed. "
            "Return what changed, what was verified, and any residual risks."
        ),
        tools=[
            make_codex_tool(
                workspace=workspace,
                sandbox_mode="workspace-write",
                codex_model=args.codex_model,
                reasoning_effort=args.reasoning_effort,
                approval_policy=args.approval_policy,
                web_search_mode=args.web_search_mode,
                network_access=args.network_access,
                idle_timeout_seconds=args.idle_timeout_seconds,
                skip_git_repo_check=skip_git_repo_check,
            )
        ],
    )

    tester = Agent(
        name="Tester",
        model=args.manager_model,
        instructions=(
            "You are the verification engineer. "
            "Use the Codex tool to run targeted tests, builds, and smoke checks for the current task. "
            "Prefer the smallest sufficient verification set. "
            "Return failing commands first if anything breaks; otherwise list the checks that passed."
        ),
        tools=[
            make_codex_tool(
                workspace=workspace,
                sandbox_mode="workspace-write",
                codex_model=args.codex_model,
                reasoning_effort=args.reasoning_effort,
                approval_policy=args.approval_policy,
                web_search_mode=args.web_search_mode,
                network_access=args.network_access,
                idle_timeout_seconds=args.idle_timeout_seconds,
                skip_git_repo_check=skip_git_repo_check,
            )
        ],
    )

    reviewer = Agent(
        name="Reviewer",
        model=args.manager_model,
        instructions=(
            "You are a strict code reviewer. "
            "Use the Codex tool to inspect the modified workspace and identify bugs, regressions, unsafe assumptions, "
            "or missing tests. Prefer findings over praise. Do not edit files."
        ),
        tools=[
            make_codex_tool(
                workspace=workspace,
                sandbox_mode="read-only",
                codex_model=args.codex_model,
                reasoning_effort=args.reasoning_effort,
                approval_policy=args.approval_policy,
                web_search_mode=args.web_search_mode,
                network_access=args.network_access,
                idle_timeout_seconds=args.idle_timeout_seconds,
                skip_git_repo_check=skip_git_repo_check,
            )
        ],
    )

    return Agent(
        name="DevelopmentManager",
        model=args.manager_model,
        instructions=(
            "You manage a multi-agent software delivery team. "
            "Always coordinate the work in this order: planner, implementer, tester, reviewer. "
            "If tester or reviewer reports a blocking issue, send the issue back to implementer once, then rerun tester "
            "and reviewer. "
            "Do not claim success unless implementation, testing, and review have all been executed. "
            "Your final answer must contain: outcome, touched areas, verification, findings or residual risks, and next steps if blocked."
        ),
        tools=[
            planner.as_tool(
                tool_name="plan_task",
                tool_description="Inspect the repository and create a concrete execution plan.",
            ),
            implementer.as_tool(
                tool_name="implement_task",
                tool_description="Make the code changes needed for the task and run focused checks.",
            ),
            tester.as_tool(
                tool_name="verify_task",
                tool_description="Run targeted tests and verification commands for the task.",
            ),
            reviewer.as_tool(
                tool_name="review_task",
                tool_description="Review the resulting changes for bugs, regressions, and missing tests.",
            ),
        ],
    )


def build_input(task: str, workspace: Path, top_level_entries: Iterable[str]) -> str:
    top_level = ", ".join(top_level_entries)
    return (
        f"Workspace: {workspace}\n"
        f"Top-level entries: {top_level}\n\n"
        "Development objective:\n"
        f"{task}\n\n"
        "Constraints:\n"
        "- Work only inside the given workspace.\n"
        "- Keep changes production-oriented and explicit.\n"
        "- Prefer targeted verification over vague claims.\n"
        "- Summarize what actually changed and what remains risky.\n"
    )


def main() -> int:
    args = parse_args()
    require_api_key()

    workspace = Path(args.workspace).expanduser().resolve()
    if not workspace.exists():
        raise SystemExit(f"Workspace does not exist: {workspace}")
    if not workspace.is_dir():
        raise SystemExit(f"Workspace is not a directory: {workspace}")

    manager = build_agents(args, workspace)
    result = Runner.run_sync(
        manager,
        input=build_input(args.task, workspace, workspace_entries(workspace)),
        context={
            "workspace": str(workspace),
            "task": args.task,
        },
        max_turns=args.max_turns,
    )
    print(result.final_output)
    return 0


if __name__ == "__main__":
    sys.exit(main())
