import importlib.util
import json
import sys
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory


SCRIPT_PATH = Path(__file__).resolve().parents[1] / "codex_cli_multi_agent_team.py"
SPEC = importlib.util.spec_from_file_location("codex_cli_multi_agent_team", SCRIPT_PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class CodexCliMultiAgentTeamTests(unittest.TestCase):
    def test_parse_task_from_prompt(self) -> None:
        prompt = """
        Workspace: /tmp/work
        Top-level entries: scripts/, README.md

        Task:
        继续开发 secure peer transport

        Output format rules:
        - First line must be exactly `STATUS: PASS` or `STATUS: FAIL`.
        """
        self.assertEqual(
            MODULE.parse_task_from_prompt(prompt),
            "继续开发 secure peer transport",
        )

    def test_select_history_entries_and_format_context(self) -> None:
        with TemporaryDirectory() as tmp:
            root = Path(tmp)
            workspace = root / "workspace"
            runs_dir = workspace / ".codex-team-runs"
            tracked_file = workspace / "clients" / "linux-agent" / "internal" / "agent" / "agent.go"
            tracked_file.parent.mkdir(parents=True, exist_ok=True)
            tracked_file.write_text("package agent\n", encoding="utf-8")
            runs_dir.mkdir(parents=True, exist_ok=True)

            matching_run = runs_dir / "20260310-100000"
            matching_run.mkdir()
            (matching_run / "implementer.prompt.txt").write_text(
                "Task:\n继续开发 secure peer transport 的 relay 回退\n\nOutput format rules:\n",
                encoding="utf-8",
            )
            (matching_run / "status.json").write_text(
                json.dumps(
                    {
                        "overall_status": "failed",
                        "current_worker": "implementer",
                        "step": "failed",
                        "message": "relay fallback test coverage missing",
                        "updated_at": "2026-03-10T08:00:00+00:00",
                    },
                    ensure_ascii=False,
                ),
                encoding="utf-8",
            )
            (matching_run / "implementer.final.txt").write_text(
                "\n".join(
                    [
                        "STATUS: PASS",
                        "",
                        "Summary",
                        "补了 secure transport 的基础实现。",
                        "",
                        "Details",
                        "clients/linux-agent/internal/agent/agent.go",
                        "",
                        "Verification",
                        "go test ./clients/linux-agent/internal/agent",
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            unrelated_run = runs_dir / "20260310-090000"
            unrelated_run.mkdir()
            (unrelated_run / "implementer.prompt.txt").write_text(
                "Task:\n修复控制面登录接口\n\nOutput format rules:\n",
                encoding="utf-8",
            )
            (unrelated_run / "status.json").write_text(
                json.dumps(
                    {
                        "overall_status": "completed",
                        "current_worker": "manager",
                        "step": "finished",
                        "message": "done",
                        "updated_at": "2026-03-10T07:00:00+00:00",
                    },
                    ensure_ascii=False,
                ),
                encoding="utf-8",
            )

            selected = MODULE.select_history_entries(
                task="继续开发 secure peer transport",
                workspace=workspace,
                runs_dir=runs_dir,
                current_run_dir=runs_dir / "20260310-110000",
                history_limit=2,
            )

            self.assertEqual(len(selected), 1)
            self.assertEqual(selected[0].run_id, "20260310-100000")
            self.assertIn("clients/linux-agent/internal/agent/agent.go", selected[0].files)

            context = MODULE.format_history_context(selected, 2000)
            self.assertIn("20260310-100000", context)
            self.assertIn("relay fallback test coverage missing", context)
            self.assertIn("clients/linux-agent/internal/agent/agent.go", context)

    def test_worker_prompt_includes_history_context(self) -> None:
        with TemporaryDirectory() as tmp:
            workspace = Path(tmp)
            (workspace / "README.md").write_text("# workspace\n", encoding="utf-8")
            prompt = MODULE.worker_prompt(
                MODULE.IMPLEMENTER,
                task="继续开发 relay fallback",
                workspace=workspace,
                history_context="Historical context from prior local multi-agent runs:\n- Run 1",
                extra_context="Plan from planner:\n\nSTATUS: PASS",
            )
            self.assertIn("Historical context from prior local multi-agent runs", prompt)
            self.assertIn("Plan from planner", prompt)


if __name__ == "__main__":
    unittest.main()
