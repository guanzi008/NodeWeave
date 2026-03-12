#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
STATE_PATH="$TMP_DIR/linux-cli-state.json"
AGENT_CONFIG="$TMP_DIR/linux-agent.json"
PORT="${NODEWEAVE_E2E_PORT:-18080}"
BASE_URL="http://127.0.0.1:${PORT}"
CONTROLPLANE_LOG="$TMP_DIR/controlplane.log"

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cd "$ROOT_DIR"

export CONTROLPLANE_ADDRESS=":${PORT}"
export CONTROLPLANE_STORAGE_DRIVER=sqlite
export CONTROLPLANE_SQLITE_PATH="$TMP_DIR/controlplane.db"
export CONTROLPLANE_ADMIN_EMAIL=admin@example.com
export CONTROLPLANE_ADMIN_PASSWORD=dev-password
export CONTROLPLANE_ADMIN_TOKEN=dev-admin-token
export CONTROLPLANE_REGISTRATION_TOKEN=dev-register-token
export CONTROLPLANE_DNS_DOMAIN=internal.net
export CONTROLPLANE_RELAYS=127.0.0.1:3478,127.0.0.1:3479

go run ./services/controlplane/cmd/controlplane >"$CONTROLPLANE_LOG" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 40); do
  if curl -fsS "${BASE_URL}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

curl -fsS "${BASE_URL}/healthz" >/dev/null

LOGIN_JSON="$(go run ./clients/linux-cli/cmd/linux-cli login --server "$BASE_URL")"
ADMIN_TOKEN="$(printf '%s' "$LOGIN_JSON" | python -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')"

python - <<PY
import json
path = "$AGENT_CONFIG"
cfg = {
    "server_url": "$BASE_URL",
    "registration_token": "dev-register-token",
    "device_name": "e2e-linux-agent",
    "platform": "linux-agent",
    "version": "0.1.0",
    "public_key": "",
    "private_key_path": "$TMP_DIR/linux-agent-private.key",
    "state_path": "$TMP_DIR/linux-agent-state.json",
    "bootstrap_path": "$TMP_DIR/linux-agent-bootstrap.json",
    "advertise_endpoints": ["198.51.100.20:51820"],
    "relay_region": "ap",
    "auto_enroll": True,
    "runtime_path": "$TMP_DIR/linux-agent-runtime.json",
    "plan_path": "$TMP_DIR/linux-agent-plan.json",
    "apply_report_path": "$TMP_DIR/linux-agent-apply-report.json",
    "session_path": "$TMP_DIR/linux-agent-session.json",
    "session_report_path": "$TMP_DIR/linux-agent-session-report.json",
    "dataplane_path": "$TMP_DIR/linux-agent-dataplane.json",
    "direct_attempt_path": "$TMP_DIR/linux-agent-direct-attempts.json",
    "direct_attempt_report_path": "$TMP_DIR/linux-agent-direct-attempt-report.json",
    "transport_report_path": "$TMP_DIR/linux-agent-transport-report.json",
    "recovery_state_path": "$TMP_DIR/linux-agent-recovery-state.json",
    "stun_report_path": "$TMP_DIR/linux-agent-stun-report.json",
    "apply_mode": "linux-plan",
    "dataplane_mode": "secure-udp",
    "dataplane_listen_address": "127.0.0.1:0",
    "tunnel_mode": "off",
    "tunnel_name": "nw0",
    "interface_name": "nw0",
    "interface_mtu": 1380,
    "exec_require_root": False,
    "exec_command_timeout": "5s",
    "session_probe_mode": "off",
    "session_listen_address": "",
    "session_probe_timeout": "1500ms",
    "heartbeat_interval": "1s",
    "bootstrap_interval": "2s",
}
with open(path, "w", encoding="utf-8") as f:
    json.dump(cfg, f, indent=2)
PY

ENROLL_JSON="$(go run ./clients/linux-cli/cmd/linux-cli enroll --server "$BASE_URL" --state "$STATE_PATH")"
NODE_ID="$(printf '%s' "$ENROLL_JSON" | python -c 'import json,sys; print(json.load(sys.stdin)["node"]["id"])')"

go run ./clients/linux-cli/cmd/linux-cli nodes --server "$BASE_URL" --token "$ADMIN_TOKEN" >/dev/null
go run ./clients/linux-cli/cmd/linux-cli route-create --server "$BASE_URL" --token "$ADMIN_TOKEN" --network 10.88.0.0/16 --via-node "$NODE_ID" --priority 100 >/dev/null
go run ./clients/linux-cli/cmd/linux-cli routes --server "$BASE_URL" --token "$ADMIN_TOKEN" >/dev/null
go run ./clients/linux-cli/cmd/linux-cli dns-zones --server "$BASE_URL" --token "$ADMIN_TOKEN" >/dev/null
go run ./clients/linux-cli/cmd/linux-cli heartbeat --state "$STATE_PATH" --endpoints 203.0.113.10:51820 --relay-region ap >/dev/null
go run ./clients/linux-cli/cmd/linux-cli status --state "$STATE_PATH" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent enroll --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent run --config "$AGENT_CONFIG" --duration 3s >/dev/null
go run ./clients/linux-agent/cmd/linux-agent status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent runtime-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent plan-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent apply-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent session-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent session-report --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent dataplane-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent transport-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent direct-attempt-status --config "$AGENT_CONFIG" >/dev/null
go run ./clients/linux-agent/cmd/linux-agent direct-attempt-report --config "$AGENT_CONFIG" >/dev/null

echo "e2e smoke passed"
