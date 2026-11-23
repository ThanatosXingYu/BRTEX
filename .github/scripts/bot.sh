#!/usr/bin/env bash
# Lightweight triage and reply bot for issues and pull requests.
# Runs inside GitHub Actions; no server involved.
#
# Triggers (see bot.yml):
#   - issue opened: exactly one triage comment per issue
#   - comment containing @artex-bot: answers that comment (works in PR
#     threads too, since issue_comment fires there)
#   - pull request opened: one first-look review comment
# The bot never posts an approving review and never replies to itself,
# to the maintainer, or to other bots (enforced in bot.yml).
set -uo pipefail

MAX_CHARS=8000
FOOTER='---
🤖 automated reply from the repo bot. A human maintainer reads everything. Ping me in any comment with `@artex-bot`.'

log() { echo "[bot] $*"; }

[ -z "${DEEPSEEK_API_KEY:-}" ] && { log "DEEPSEEK_API_KEY secret is not set; skipping."; exit 0; }

ev=$(cat "$GITHUB_EVENT_PATH")
event_name="$GITHUB_EVENT_NAME"
repo="$GITHUB_REPOSITORY"

ask_model() { # $1 = system prompt, $2 = user payload, $3 = "json" for JSON mode; prints the answer
  local body="${2:0:$MAX_CHARS}" extra
  local req resp
  extra='{}'
  [ "${3:-}" = "json" ] && extra='{ "response_format": { "type": "json_object" } }'
  req=$(jq -n --arg sys "$1" --arg body "$body" --argjson extra "$extra" \
    '{model: "deepseek-flash",
      messages: [{role: "system", content: $sys}, {role: "user", content: $body}],
      max_tokens: 900, temperature: 0} * $extra')
  resp=$(curl -sS --max-time 120 https://api.deepseek.com/chat/completions \
    -H "Authorization: Bearer $DEEPSEEK_API_KEY" \
    -H 'Content-Type: application/json' \
    -d "$req")
  if ! jq -e '.choices[0].message.content' <<<"$resp" >/dev/null 2>&1; then
    log "api call failed, raw response: ${resp:0:300}"
    echo ""
    return
  fi
  jq -r '.choices[0].message.content' <<<"$resp"
}

# repo_context prints a bounded, structural view of the tree: the root files,
# every directory with its file count, and the paths a question is usually
# about. It is a summary, not a listing, and the prompts say so: an earlier
# version printed the first 120 paths alphabetically, which stopped at config/
# and made the bot assert that docs/ and scripts/benchmark/ do not exist.
repo_context() {
  local total
  total=$(git ls-files | wc -l | tr -d ' ')
  echo "Tracked files: $total. The directory map below is complete, the file list is not: never conclude that a path does not exist because it is absent from this summary."
  echo
  echo "Root files:"
  git ls-files | awk -F/ 'NF == 1 { print "  " $1 }'
  echo
  echo "Top-level directories, with every file beneath them:"
  git ls-files | awk -F/ 'NF >= 2 { n[$1]++ }
    END { for (k in n) printf "  %s/ %d\n", k, n[k] }' | sort
  echo
  echo "Second level, for the larger ones:"
  git ls-files | awk -F/ 'NF > 2 { n[$1 "/" $2]++ }
    END { for (k in n) printf "  %s/ %d\n", k, n[k] }' | sort
  echo
  echo "Workflows, migrations, docs and scripts:"
  git ls-files '.github/workflows/*' 'db/migrations/*' 'docs/*' 'scripts/*' | sed 's/^/  /'
}

post_comment() { # $1 = issue or PR number, $2 = markdown text
  { echo "$2"; echo; echo "$FOOTER"; } > /tmp/bot-comment.md
  if gh issue view "$1" --json pull_request --jq '.pull_request' | grep -q .; then
    gh pr comment "$1" --body-file /tmp/bot-comment.md
  else
    gh issue comment "$1" --body-file /tmp/bot-comment.md
  fi
}

eyes() { # $1 = issue or PR number
  gh api -X POST "repos/$repo/issues/$1/reactions" -f content=eyes >/dev/null 2>&1 || true
}

# Labels the triage may request; anything else the model names is dropped.
LABEL_COLORS=(bug=d73a4a question=d876e3 feature=a2eeef P1=b60205 P2=fbca04 P3=c5def5)

ensure_label() { # $1 = label name
  local entry color
  for entry in "${LABEL_COLORS[@]}"; do
    [ "${entry%%=*}" = "$1" ] && color="${entry#*=}"
  done
  gh label create "$1" --color "${color:-cccccc}" --force >/dev/null 2>&1 || true
}

apply_labels() { # $1 = issue number, $2 = comma separated label names from the model
  local wanted=() item ok w
  IFS=',' read -ra parts <<<"$2"
  for item in "${parts[@]}"; do
    item=$(echo "$item" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
    ok=0
    for w in bug question feature p1 p2 p3; do [ "$item" = "$w" ] && ok=1; done
    [ "$ok" = 1 ] || continue
    case "$item" in p1|p2|p3) item=${item^^} ;; esac
    wanted+=("$item")
  done
  if [ "${#wanted[@]}" -gt 0 ]; then
    for item in "${wanted[@]}"; do ensure_label "$item"; done
    gh issue edit "$1" --add-label "$(IFS=,; echo "${wanted[*]}")" >/dev/null 2>&1 || log "label apply failed"
    log "applied labels: ${wanted[*]}"
  fi
}

already_triaged() { # $1 = issue number
  gh api "repos/$repo/issues/$1/comments" --jq '.[].body' 2>/dev/null \
    | grep -q 'automated reply from the repo bot'
}

answer=""

case "$event_name" in
  issues)
    number=$(jq -r '.issue.number' <<<"$ev")
    title=$(jq -r '.issue.title' <<<"$ev")
    body=$(jq -r '.issue.body // ""' <<<"$ev")
    maintainer_opened=false
    [ "$GITHUB_ACTOR" = "BaaRai" ] && maintainer_opened=true
    if [ "$maintainer_opened" = false ] && already_triaged "$number"; then log "issue #$number already triaged"; exit 0; fi
    [ "$maintainer_opened" = false ] && eyes "$number"
    answer=$(ask_model \
      'You are the triage bot of an open-source Go + Next.js repository (an autonomous penetration testing agent system; AGPL-3.0). Respond ONLY with a single JSON object, no markdown, no prose outside the JSON, exactly of this shape: {"comment": "<your triage comment, max 180 words, same language as the report>", "labels": ["<zero or more of: bug, question, feature, P1, P2, P3>"]}. Classify the report as bug, question, or feature request; if it is a question, answer it concisely using the repository file list as context in the comment; if it is a bug report, the comment says what reproduction steps or version information are missing; if it is a feature request, the comment restates it in one sentence; add a priority label (P1 security impact or data loss, P2 core functionality broken, P3 minor or cosmetic) only for bugs; use an empty labels array if nothing fits. The repository summary lists directories with counts and a few key paths, not every file: never state that a file, directory or feature does not exist because it is missing from it, say you are not sure instead.' \
      "Issue title: $title

Issue body:
$body

Repository file list:
$(repo_context)" json)
    [ -z "$answer" ] && { log "model returned nothing"; exit 0; }
    log "raw json: $answer"
    comment=$(jq -r '.comment // empty' <<<"$answer" 2>/dev/null)
    reply_labels=$(jq -r 'if (.labels | type) == "array" then (.labels | map(tostring) | join(",")) else "" end' <<<"$answer" 2>/dev/null)
    [ -z "$reply_labels" ] && reply_labels="none"
    log "parsed labels: $reply_labels"
    if [ "$maintainer_opened" = true ]; then
      log "maintainer issue #$number: labels only, staying silent"
      apply_labels "$number" "$reply_labels"
      exit 0
    fi
    if [ -n "$comment" ]; then
      post_comment "$number" "$comment"
    else
      log "no comment field in JSON; labels only"
    fi
    apply_labels "$number" "$reply_labels"
    ;;

  issue_comment)
    number=$(jq -r '.issue.number' <<<"$ev")
    body=$(jq -r '.comment.body' <<<"$ev")
    question="${body#*@artex-bot}"            # strip up to the mention
    echo "$body" | grep -qi '@artex-bot' || { log "no mention; skipping"; exit 0; }
    eyes "$number"
    answer=$(ask_model \
      'You are the helper bot of an open-source Go + Next.js repository (an autonomous penetration testing agent system; AGPL-3.0). Someone pinged you with a question about this repository. Answer it concisely from the repository summary, saying plainly when you are not sure and suggesting a proper issue. Never state that a file, directory or feature does not exist because it is missing from that summary: it lists directories, not every file. Maximum 180 words, plain markdown, reply in the language of the question.' \
      "Question:
$question

Repository file list:
$(repo_context)")
    [ -z "$answer" ] && { log "model returned nothing"; exit 0; }
    post_comment "$number" "$answer"
    ;;

  pull_request)
    number=$(jq -r '.pull_request.number' <<<"$ev")
    title=$(jq -r '.pull_request.title' <<<"$ev")
    body=$(jq -r '.pull_request.body // ""' <<<"$ev")
    eyes "$number"
    diff=$(gh pr diff "$number" 2>/dev/null | head -c 4000)
    answer=$(ask_model \
      'You are the first-look bot of an open-source Go + Next.js repository (an autonomous penetration testing agent system; AGPL-3.0). Summarize what this pull request changes in two or three sentences, then a short checklist: does it include or update tests, does it touch documentation where needed, does anything look inconsistent with the described goal. You are commenting for information only; you do not approve or request changes. Maximum 180 words, plain markdown.' \
      "Pull request title: $title

Pull request description:
$body

Diff (truncated):
$diff")
    [ -z "$answer" ] && { log "model returned nothing"; exit 0; }
    post_comment "$number" "$answer"
    ;;

  *)
    log "unhandled event $event_name; skipping"
    ;;
esac

log "done"
