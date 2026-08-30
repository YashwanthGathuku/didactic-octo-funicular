#!/usr/bin/env bash
set -eu
set -o pipefail 2>/dev/null || true

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "============================================================"
echo " SentinelFlow submission hardening freeze — LOCAL ONLY"
echo "============================================================"
echo "No cloud deployment or production mutation is performed."

GO_BIN="go"
if ! command -v go >/dev/null 2>&1 && command -v go.exe >/dev/null 2>&1; then
  GO_BIN="go.exe"
fi

PYTHON_BIN="python"
if command -v python.exe >/dev/null 2>&1; then
  PYTHON_BIN="python.exe"
elif command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN="python3"
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN="python"
fi

PYTEST_BIN="pytest"
if ! command -v pytest >/dev/null 2>&1 && command -v pytest.exe >/dev/null 2>&1; then
  PYTEST_BIN="pytest.exe"
elif ! command -v pytest >/dev/null 2>&1; then
  PYTEST_BIN="$PYTHON_BIN -m pytest"
fi

NPM_BIN="npm"
if ! command -v npm >/dev/null 2>&1 && command -v npm.cmd >/dev/null 2>&1; then
  NPM_BIN="npm.cmd"
fi

# ---------------------------------------------------------------------------
# Submission eligibility / P11 identity truth checks
# ---------------------------------------------------------------------------
echo "[1/12] Executable model-version guard"
if grep -RInE --exclude-dir=.git --exclude='*.md' --exclude='*.json' --exclude='*.yaml' --exclude='*.yml' \
  'gemini-(1\.5|2\.0|2\.5)' "$ROOT/ai-tier"; then
  echo "ERROR: legacy Gemini model found in executable AI-tier source" >&2
  exit 1
fi

# Do not use raw grep for the legacy identity header: comments and docstrings are
# documentation, not executable behavior. Parse Python and reject the exact
# legacy header literal only when it appears in executable AST nodes.
$PYTHON_BIN - "$ROOT/ai-tier/runtime" <<'PY'
import ast
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
needle = "X-Agent-Identity-Principal"
violations: list[str] = []


def body_without_docstring(body):
    if (
        body
        and isinstance(body[0], ast.Expr)
        and isinstance(body[0].value, ast.Constant)
        and isinstance(body[0].value.value, str)
    ):
        return body[1:]
    return body


class ExecutableStringVisitor(ast.NodeVisitor):
    def __init__(self, path: pathlib.Path):
        self.path = path

    def visit_Module(self, node: ast.Module):
        for stmt in body_without_docstring(node.body):
            self.visit(stmt)

    def visit_FunctionDef(self, node: ast.FunctionDef):
        for decorator in node.decorator_list:
            self.visit(decorator)
        for default in (*node.args.defaults, *node.args.kw_defaults):
            if default is not None:
                self.visit(default)
        for stmt in body_without_docstring(node.body):
            self.visit(stmt)

    visit_AsyncFunctionDef = visit_FunctionDef

    def visit_ClassDef(self, node: ast.ClassDef):
        for decorator in node.decorator_list:
            self.visit(decorator)
        for base in node.bases:
            self.visit(base)
        for keyword in node.keywords:
            self.visit(keyword)
        for stmt in body_without_docstring(node.body):
            self.visit(stmt)

    def visit_Constant(self, node: ast.Constant):
        if node.value == needle:
            violations.append(f"{self.path}:{node.lineno}")


for path in sorted(root.rglob("*.py")):
    try:
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    except (OSError, SyntaxError) as exc:
        print(f"ERROR: unable to parse {path}: {exc}", file=sys.stderr)
        sys.exit(1)
    ExecutableStringVisitor(path).visit(tree)

if violations:
    print("ERROR: runtime code still manufactures/references the legacy identity-principal header", file=sys.stderr)
    for violation in violations:
        print(violation, file=sys.stderr)
    sys.exit(1)
PY

if grep -RIn --exclude='test_*' --exclude='*_test.py' '"status": "COMPLETED"' "$ROOT/ai-tier/runtime"; then
  echo "ERROR: runtime adapter appears to claim COMPLETED without managed proof" >&2
  exit 1
fi

echo "[2/12] Go P11.5 managed-ingress authentication"
(
  cd "$ROOT/gateway"
  $GO_BIN test ./internal/auth/...
)

echo "[3/12] Python P11.5 managed runtime packaging"
(
  cd "$ROOT"
  $PYTEST_BIN ai-tier/tests/test_platform_runtime.py -v
)

echo "[4/12] Managed Agent Runtime deployment dry-run"
(
  cd "$ROOT/ai-tier"
  $PYTHON_BIN -m runtime.deploy_agent_runtime \
    --project "${GOOGLE_CLOUD_PROJECT:-telos-agent}" \
    --location "${GOOGLE_CLOUD_LOCATION:-us-central1}"
)

echo "[5/12] P12.5 deterministic return-risk gate"
(
  cd "$ROOT/gateway"
  $GO_BIN test ./internal/returnrisk/...
)
(
  cd "$ROOT"
  $PYTEST_BIN ai-tier/tests/test_return_risk_agent.py -v
  $PYTHON_BIN ai-tier/evals/return_runner.py
)

echo "[6/12] P13-P15 execution-control + Tool Gateway race gate"
(
  cd "$ROOT/gateway"
  $GO_BIN test ./internal/executioncontrol/... ./internal/toolgateway/...
)

echo "[7/12] Full Go internal regression"
(
  cd "$ROOT/gateway"
  $GO_BIN test ./internal/...
)

echo "[8/12] Full Python AI-tier regression"
(
  cd "$ROOT"
  $PYTEST_BIN ai-tier/tests/ -v
)

echo "[9/12] Master adversarial evaluation"
(
  cd "$ROOT"
  $PYTHON_BIN ai-tier/evals/runner.py
)

echo "[10/12] Frontend unit tests"
(
  cd "$ROOT"
  $NPM_BIN test -- --run
)

echo "[11/12] Frontend production build"
(
  cd "$ROOT"
  $NPM_BIN run build
)

echo "[12/12] Documentation & Registry synchronization"
(
  cd "$ROOT"
  $PYTHON_BIN scripts/generate_docs.py
  $PYTHON_BIN scripts/generate_docs.py --check
  $PYTHON_BIN scripts/validate_agent_registry.py --check
)

echo "============================================================"
echo " SUBMISSION HARDENING FREEZE LOCAL GATE PASSED"
echo "============================================================"
echo "Live Agent Runtime / Agent Identity / Agent Gateway / Registry /"
echo "Memory Bank / Model Armor service / Cloud Trace remain separate"
echo "PASS_LIVE gates until actual Google Cloud evidence is captured."
