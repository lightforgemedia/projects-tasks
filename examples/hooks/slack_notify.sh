#!/usr/bin/env bash
set -euo pipefail
# Mock notification; replace echo with curl to a webhook.
echo "Notify: ${PT_EVENT} ${PT_ID} status ${PT_STATUS_TO} assignee ${PT_ASSIGNEE} actor ${PT_ACTOR}" >> /tmp/pt-slack.log
