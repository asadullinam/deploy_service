#!/usr/bin/env bash
set -euo pipefail

if [ -z "${TELEGRAM_BOT_TOKEN:-}" ]; then
  echo "telegram bot token is empty, skipping admin notification"
  exit 0
fi

if [ -z "${TELEGRAM_MESSAGE:-}" ]; then
  echo "telegram message is empty, skipping admin notification"
  exit 0
fi

namespace="${NAMESPACE:-deploy-service}"
postgres_pod="${POSTGRES_POD_NAME:-deploy-service-postgres-0}"

normalize_username() {
  local raw="$1"
  raw="${raw#"@"}"
  printf '%s' "$raw" | tr '[:upper:]' '[:lower:]'
}

usernames=()
chat_ids=()

if [ -n "${TELEGRAM_ADMIN_CHAT_IDS:-}" ]; then
  while IFS= read -r part || [ -n "$part" ]; do
    value="$(printf '%s' "$part" | xargs)"
    if [ -n "$value" ]; then
      chat_ids+=("$value")
    fi
  done < <(printf '%s' "$TELEGRAM_ADMIN_CHAT_IDS" | tr ',' '\n')
fi

if [ -n "${TELEGRAM_ADMIN_USERNAMES:-}" ]; then
  while IFS= read -r part || [ -n "$part" ]; do
    value="$(printf '%s' "$part" | xargs)"
    if [ -n "$value" ]; then
      usernames+=("$(normalize_username "$value")")
    fi
  done < <(printf '%s' "$TELEGRAM_ADMIN_USERNAMES" | tr ',' '\n')
fi

if [ "${#usernames[@]}" -gt 0 ] && kubectl -n "$namespace" get pod "$postgres_pod" >/dev/null 2>&1; then
  quoted_usernames=()
  for username in "${usernames[@]}"; do
    escaped="${username//\'/\'\'}"
    quoted_usernames+=("'${escaped}'")
  done
  csv="$(IFS=,; printf '%s' "${quoted_usernames[*]}")"
  query="select lower(telegram_username), telegram_chat_id from users where telegram_chat_id <> 0 and lower(telegram_username) in (${csv});"
  rows="$(kubectl -n "$namespace" exec "$postgres_pod" -- sh -lc "psql -U deploy -d deploy_service -AtF '|' -c \"$query\"" || true)"
  while IFS='|' read -r _username chat_id; do
    if [ -n "$chat_id" ]; then
      chat_ids+=("$chat_id")
    fi
  done <<< "$rows"
elif [ "${#usernames[@]}" -gt 0 ]; then
  echo "postgres pod $postgres_pod is unavailable, using direct admin chat id fallback only"
fi

if [ "${#chat_ids[@]}" -eq 0 ]; then
  echo "no admin chats resolved, skipping admin notification"
  exit 0
fi

sent_count=0
seen_chat_ids=""
for chat_id in "${chat_ids[@]}"; do
  case "
$seen_chat_ids
" in
    *"
$chat_id
"*)
      continue
      ;;
  esac
  seen_chat_ids="${seen_chat_ids}
${chat_id}"
  curl --silent --show-error --fail \
    --request POST \
    --url "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
    --data-urlencode "chat_id=${chat_id}" \
    --data-urlencode "text=${TELEGRAM_MESSAGE}" \
    >/dev/null
  sent_count=$((sent_count + 1))
done

echo "sent admin telegram notification to ${sent_count} chat(s)"
