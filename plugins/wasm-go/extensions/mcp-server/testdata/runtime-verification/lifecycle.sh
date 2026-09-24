#!/usr/bin/env bash
# Shared bounded lifecycle gates. The caller provides compose_runtime().

runtime_sleep() {
  sleep "$1"
}

runtime_diagnostic() {
  message=$1
  echo "$message" >&2
  if test -n "${RUNTIME_LIFECYCLE_DIAGNOSTICS:-}"; then
    echo "$message" >>"$RUNTIME_LIFECYCLE_DIAGNOSTICS"
  fi
}

runtime_service_running() {
  service=$1
  container_id=$(compose_runtime ps -a -q "$service" 2>/dev/null) || return 2
  test -n "$container_id" || return 2
  running=$(podman inspect "$container_id" --format '{{.State.Running}}' 2>/dev/null) || return 2
  case "$running" in
    true) return 0 ;;
    false) return 1 ;;
    *) return 2 ;;
  esac
}

stop_runtime_service() {
  service=$1
  attempts=${RUNTIME_STOP_ATTEMPTS:-20}
  stop_status=0
  compose_runtime stop -t 10 "$service" >/dev/null 2>&1 || stop_status=$?
  attempt=1
  while test "$attempt" -le "$attempts"; do
    running_status=0
    runtime_service_running "$service" || running_status=$?
    if test "$running_status" -eq 1; then
      return 0
    fi
    runtime_sleep 0.5
    attempt=$((attempt + 1))
  done
  runtime_diagnostic "failed to confirm $service stopped after compose status $stop_status and $attempts inspections"
  return 1
}

backend_health_probe() {
  compose_runtime exec -T backend-primary wget -q -O /dev/null http://127.0.0.1:8080/healthz
}

backend_reset_probe() {
  compose_runtime exec -T backend-primary wget -q -O /dev/null \
    --post-data '{}' http://127.0.0.1:8080/__reset
}

wait_primary_backend_ready() {
  attempts=${RUNTIME_BACKEND_HEALTH_ATTEMPTS:-60}
  attempt=1
  while test "$attempt" -le "$attempts"; do
    if backend_health_probe; then
      return 0
    fi
    runtime_sleep 0.5
    attempt=$((attempt + 1))
  done
  runtime_diagnostic "backend-primary remained unavailable after $attempts health probes"
  return 1
}

reset_primary_backend() {
  wait_primary_backend_ready || return 1
  attempts=${RUNTIME_BACKEND_RESET_ATTEMPTS:-20}
  attempt=1
  while test "$attempt" -le "$attempts"; do
    if backend_reset_probe; then
      return 0
    fi
    runtime_sleep 0.5
    attempt=$((attempt + 1))
  done
  runtime_diagnostic "backend-primary was healthy but reset failed after $attempts attempts"
  return 1
}

start_runtime_service() {
  compose_runtime up -d "$1"
}

# Dependencies supplied by run.sh and replaced by lifecycle_self_test.sh:
# wait_for_rejection_markers, capture_gateway_log, capture_empty_primary_backend.
run_static_rejection_phase() {
  service=$1
  log_path=$2
  state_path=$3
  first_marker=$4
  second_marker=$5
  reset_primary_backend || return 1
  start_runtime_service "$service" || return 1
  marker_status=0
  wait_for_rejection_markers "$service" "$first_marker" "$second_marker" || marker_status=$?
  stop_runtime_service "$service" || return 1
  capture_gateway_log "$service" "$log_path" || return 1
  capture_empty_primary_backend "$state_path" || return 1
  return "$marker_status"
}

CORPUS_VERIFY_FAILED=10
CORPUS_LIFECYCLE_FAILED=20

# run_corpus_verifier is supplied by run.sh and returns the verifier status.
run_corpus_phase() {
  revision=$1
  log_path=$2
  reset_primary_backend || return "$CORPUS_LIFECYCLE_FAILED"
  start_runtime_service "gateway-corpus-$revision" || return "$CORPUS_LIFECYCLE_FAILED"
  verifier_status=0
  run_corpus_verifier "$revision" || verifier_status=$?
  stop_runtime_service "gateway-corpus-$revision" || return "$CORPUS_LIFECYCLE_FAILED"
  capture_gateway_log "gateway-corpus-$revision" "$log_path" || return "$CORPUS_LIFECYCLE_FAILED"
  test "$verifier_status" -eq 0 || return "$CORPUS_VERIFY_FAILED"
}

run_corpus_revisions() {
  evidence_dir=$1
  aggregate_status=0
  for revision in candidate affected oracle; do
    phase_status=0
    run_corpus_phase "$revision" "$evidence_dir/gateway-corpus-$revision.log" || phase_status=$?
    case "$phase_status" in
      0) ;;
      "$CORPUS_VERIFY_FAILED") aggregate_status=$CORPUS_VERIFY_FAILED ;;
      *) return "$CORPUS_LIFECYCLE_FAILED" ;;
    esac
  done
  return "$aggregate_status"
}
