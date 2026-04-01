#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mode="files"
if [[ "${1:-}" == "--all" ]]; then
  mode="all"
  shift
elif [[ "${1:-}" == "--staged" ]]; then
  mode="staged"
  shift
fi

collect_files() {
  case "$mode" in
    all)
      git ls-files
      ;;
    staged)
      git diff --cached --name-only --diff-filter=ACMR
      ;;
    files)
      printf '%s\n' "$@"
      ;;
  esac
}

input_files=()
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  input_files+=("$line")
done < <(collect_files "$@" | sed '/^$/d')

if [[ "${#input_files[@]}" -eq 0 ]]; then
  exit 0
fi

go_files=()
prettier_files=()
sql_files=()
gomod_dirs=()

add_unique_dir() {
  local candidate="$1"
  local existing
  for existing in "${gomod_dirs[@]:-}"; do
    if [[ "$existing" == "$candidate" ]]; then
      return
    fi
  done
  gomod_dirs+=("$candidate")
}

for file in "${input_files[@]}"; do
  [[ -f "$file" ]] || continue
  case "$file" in
    *.go)
      go_files+=("$file")
      ;;
    */go.mod|go.mod)
      add_unique_dir "$(dirname "$file")"
      ;;
    *.md|*.yaml|*.yml|*.js|*.css|*.html)
      prettier_files+=("$file")
      ;;
    *.sql)
      sql_files+=("$file")
      ;;
  esac
done

if [[ "${#go_files[@]}" -gt 0 ]]; then
  gofmt -w "${go_files[@]}"
fi

if [[ "${#gomod_dirs[@]}" -gt 0 ]]; then
  for dir in "${gomod_dirs[@]}"; do
    (
      cd "$repo_root/$dir"
      go mod edit -fmt
    )
  done
fi

if [[ "${#prettier_files[@]}" -gt 0 ]]; then
  npx --no-install prettier --write "${prettier_files[@]}"
fi

if [[ "${#sql_files[@]}" -gt 0 ]]; then
  for file in "${sql_files[@]}"; do
    npx --no-install sql-formatter --fix "$file"
  done
fi

if [[ "$mode" == "staged" ]]; then
  staged_files=()
  if [[ "${#go_files[@]}" -gt 0 ]]; then
    staged_files+=("${go_files[@]}")
  fi
  if [[ "${#prettier_files[@]}" -gt 0 ]]; then
    staged_files+=("${prettier_files[@]}")
  fi
  if [[ "${#sql_files[@]}" -gt 0 ]]; then
    staged_files+=("${sql_files[@]}")
  fi
  if [[ "${#input_files[@]}" -gt 0 ]]; then
    staged_files+=("${input_files[@]}")
  fi
  if [[ "${#staged_files[@]}" -gt 0 ]]; then
    git add -- "${staged_files[@]}"
  fi
fi
