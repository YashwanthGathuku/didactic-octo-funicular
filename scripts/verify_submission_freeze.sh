#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "============================================================"
echo " SentinelFlow submission hardening freeze — LOCAL ONLY"
echo "============================================================"
echo "No cloud deployment or production mutation is performed."

if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: go is required" >&2
  exit 1
fi
if ! command -v python >/dev/null 2>&1; then
  echo "ERROR: python is required" >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "ERROR: npm is required" >&2
  exit 1
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
python - "$ROOT/ai-tier/runtime" <<'PY'
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
  go test -race ./internal/auth/...
)

echo "[3/12] Python P11.5 managed runtime packaging"
(
  cd "$ROOT"
  pytest ai-tier/tests/test_platform_runtime.py -v
)

echo "[4/12] Managed Agent Runtime deployment dry-run"
(
  cd "$ROOT/ai-tier"
  python -m runtime.deploy_agent_runtime \
    --project "${GOOGLE_CLOUD_PROJECT:-telos-agent}" \
    --location "${GOOGLE_CLOUD_LOCATION:-us-central1}"
)

echo "[5/12] P12.5 deterministic return-risk gate"
(
  cd "$ROOT/gateway"
  go test -race ./internal/returnrisk/...
)
(
  cd "$ROOT"
  pytest ai-tier/tests/test_return_risk_agent.py -v
  python ai-tier/evals/return_runner.py
)

echo "[6/12] P13-P15 execution-control + Tool Gateway race gate"
(
  cd "$ROOT/gateway"
  go test -race ./internal/executioncontrol/... ./internal/toolgateway/...
)

echo "[7/12] Full Go internal regression"
(
  cd "$ROOT/gateway"
  go test -race ./internal/...
)

echo "[8/12] Full Python AI-tier regression"
(
  cd "$ROOT"
  pytest ai-tier/tests/ -v
)

echo "[9/12] Master adversarial evaluation"
(
  cd "$ROOT"
  python ai-tier/evals/runner.py
)

echo "[10/12] Frontend unit tests"
(
  cd "$ROOT"
  npm test -- --run
)

echo "[11/12] Frontend production build"
(
  cd "$ROOT"
  npm run build
)

echo "[12/12] Generated documentation synchronization"
(
  cd "$ROOT"
  python scripts/generate_docs.py
  python scripts/generate_docs.py --check
)

echo "============================================================"
echo " SUBMISSION HARDENING FREEZE LOCAL GATE PASSED"
echo "============================================================"
echo "Live Agent Runtime / Agent Identity / Agent Gateway / Registry /"
echo "Memory Bank / Model Armor service / Cloud Trace remain separate"
echo "PASS_LIVE gates until actual Google Cloud evidence is captured."
