#!/bin/bash

die() {
  echo "Error: $*" >&2
  exit 1
}

require_args_count() {
  local got="$1"
  local expected="$2"
  local usage="$3"
  [ "$got" -eq "$expected" ] || die "$usage"
}

require_nonempty() {
  local value="$1"
  local name="$2"
  [ -n "$value" ] || die "$name is empty"
}

require_bool() {
  local value="$1"
  local name="$2"
  case "$value" in
    true|false) ;;
    *) die "Invalid $name: $value (expected true/false)" ;;
  esac
}

require_env() {
  local name="$1"
  [ -n "${!name:-}" ] || die "$name is not set"
}
