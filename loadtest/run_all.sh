#!/bin/bash
set -e

# Base URL targeting the API server running on the host
BASE_URL="http://host.docker.internal:8080"

run_test() {
  local script_path=$1
  echo "=================================================================="
  echo "🚀 Running load test: ${script_path}"
  echo "=================================================================="
  docker run --rm -i \
    --add-host=host.docker.internal:host-gateway \
    -e PAYCORE_BASE_URL="${BASE_URL}" \
    -e PAYCORE_LOADTEST_DURATION="${PAYCORE_LOADTEST_DURATION}" \
    -e PAYCORE_LOADTEST_VUS="${PAYCORE_LOADTEST_VUS}" \
    -v "$(pwd)":/app \
    -w /app \
    grafana/k6 run "${script_path}"
  echo "=================================================================="
  echo "✅ Finished: ${script_path}"
  echo "=================================================================="
  echo ""
}

run_test "loadtest/payment_happy_path.js"
run_test "loadtest/idempotency_replay.js"
run_test "loadtest/rate_limit_pressure.js"
run_test "loadtest/payer_contention.js"
run_test "loadtest/settlement_outbox_backlog.js"
