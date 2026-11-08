#!/bin/bash
# Higress Daily Report Generator
# Generates daily report for alibaba/higress repository

# set -e  # 临时禁用以调试

REPO="alibaba/higress"
CHANNEL="1465549185632702591"
DATE=$(date +"%Y-%m-%d")
REPORT_DIR="/root/clawd/reports"
TRACKING_DIR="/root/clawd/memory"
RECORD_FILE="${TRACKING_DIR}/higress-issue-process-record.md"

mkdir -p "$REPORT_DIR" "$TRACKING_DIR"

echo "=== Higress Daily Report - $DATE ==="

# Get yesterday's date
YESTERDAY=$(date -d "yesterday" +"%Y-%m-%d" 2>/dev/null || date -v-1d +"%Y-%m-%d")

echo "Fetching issues created on $YESTERDAY..."

# Fetch issues created yesterday
ISSUES=$(gh search issues --repo "${REPO}" --state open --created "${YESTERDAY}..${YESTERDAY}" --json number,title,labels,author,url,body,state --limit 50 2>/dev/null)

if [ -z "$ISSUES" ]; then
    ISSUES_COUNT=0
else
    ISSUES_COUNT=$(echo "$ISSUES" | jq 'length' 2>/dev/null || echo "0")
fi

# Fetch PRs created yesterday
PRS=$(gh search prs --repo "${REPO}" --state open --created "${YESTERDAY}..${YESTERDAY}" --json number,title,labels,author,url,reviewDecision,additions,deletions,body,state --limit 50 2>/dev/null)

if [ -z "$PRS" ]; then
    PRS_COUNT=0
else
    PRS_COUNT=$(echo "$PRS" | jq 'length' 2>/dev/null || echo "0")
fi

echo "Found: $ISSUES_COUNT issues, $PRS_COUNT PRs"

# Build report
REPORT="📊 **Higress 项目每日报告 - ${DATE}**

**📋 概览**
- 统计时间: ${YESTERDAY} 全天
- 新增 Issues: **${ISSUES_COUNT}** 个
- 新增 PRs: **${PRS_COUNT}** 个

---

"

# Process issues
if [ "$ISSUES_COUNT" -gt 0 ]; then
    REPORT="${REPORT}**📌 Issues 详情**

"

    # Use a temporary file to avoid subshell variable scoping issues
    ISSUE_DETAILS=$(mktemp)

    echo "$ISSUES" | jq -r '.[] | @json' | while IFS= read -r ISSUE; do
        NUM=$(echo "$ISSUE" | jq -r '.number')
        TITLE=$(echo "$ISSUE" | jq -r '.title')
        URL=$(echo "$ISSUE" | jq -r '.url')
        AUTHOR=$(echo "$ISSUE" | jq -r '.author.login')
        BODY=$(echo "$ISSUE" | jq -r '.body // ""')
        LABELS=$(echo "$ISSUE" | jq -r '.labels[]?.name // ""' | head -1)

        # Determine emoji
        EMOJI="📝"
        echo "$LABELS" | grep -q "priority/high" && EMOJI="🔴"
        echo "$LABELS" | grep -q "type/bug" && EMOJI="🐛"
        echo "$LABELS" | grep -q "type/enhancement" && EMOJI="✨"

        # Extract content
        CONTENT=$(echo "$BODY" | head -n 8 | sed 's/```.*```//g' | sed 's/`//g' | tr '\n' ' ' | head -c 300)

        if [ -z "$CONTENT" ]; then
            CONTENT="无详细描述"
        fi

        if [ ${#CONTENT} -eq 300 ]; then
            CONTENT="${CONTENT}..."
        fi

        # Append to temporary file
        echo "${EMOJI} **[#${NUM}](${URL})**: ${TITLE}
 👤 @${AUTHOR}
 📝 ${CONTENT}
" >> "$ISSUE_DETAILS"
    done

    # Read from temp file and append to REPORT
    REPORT="${REPORT}$(cat $ISSUE_DETAILS)"

    rm -f "$ISSUE_DETAILS"
fi

REPORT="${REPORT}
---

"

# Process PRs
if [ "$PRS_COUNT" -gt 0 ]; then
    REPORT="${REPORT}**🔀 PRs 详情**

"

    # Use a temporary file to avoid subshell variable scoping issues
    PR_DETAILS=$(mktemp)

    echo "$PRS" | jq -r '.[] | @json' | while IFS= read -r PR; do
        NUM=$(echo "$PR" | jq -r '.number')
        TITLE=$(echo "$PR" | jq -r '.title')
        URL=$(echo "$PR" | jq -r '.url')
        AUTHOR=$(echo "$PR" | jq -r '.author.login')
        ADDITIONS=$(echo "$PR" | jq -r '.additions')
        DELETIONS=$(echo "$PR" | jq -r '.deletions')
        REVIEW=$(echo "$PR" | jq -r '.reviewDecision // "pending"')
        BODY=$(echo "$PR" | jq -r '.body // ""')

        # Determine status
        STATUS="👀"
        [ "$REVIEW" = "APPROVED" ] && STATUS="✅"
        [ "$REVIEW" = "CHANGES_REQUESTED" ] && STATUS="🔄"

        # Calculate size
        TOTAL=$((ADDITIONS + DELETIONS))
        SIZE="M"
        [ $TOTAL -lt 100 ] && SIZE="XS"
        [ $TOTAL -lt 500 ] && SIZE="S"
        [ $TOTAL -lt 1000 ] && SIZE="M"
        [ $TOTAL -lt 5000 ] && SIZE="L"
        [ $TOTAL -ge 5000 ] && SIZE="XL"

        # Extract content
        CONTENT=$(echo "$BODY" | head -n 8 | sed 's/```.*```//g' | sed 's/`//g' | tr '\n' ' ' | head -c 300)

        if [ -z "$CONTENT" ]; then
            CONTENT="无详细描述"
        fi

        if [ ${#CONTENT} -eq 300 ]; then
            CONTENT="${CONTENT}..."
        fi

        # Append to temporary file
        echo "${STATUS} **[#${NUM}](${URL})**: ${TITLE} ${SIZE}
 👤 @${AUTHOR} | ${STATUS} | 变更: +${ADDITIONS}/-${DELETIONS}
 📝 ${CONTENT}
" >> "$PR_DETAILS"
    done

    # Read from temp file and append to REPORT
    REPORT="${REPORT}$(cat $PR_DETAILS)"

    rm -f "$PR_DETAILS"
fi

# Check for new comments on tracked issues
TRACKING_FILE="${TRACKING_DIR}/higress-issue-tracking.json"

echo ""
echo "Checking for new comments on tracked issues..."

# Load previous tracking data
if [ -f "$TRACKING_FILE" ]; then
    PREV_TRACKING=$(cat "$TRACKING_FILE")
    PREV_ISSUES=$(echo "$PREV_TRACKING" | jq -r '.issues[]?.number // empty' 2>/dev/null)

    if [ -n "$PREV_ISSUES" ]; then
        REPORT="${REPORT}**🔔 Issue跟进（新评论）**"

        HAS_NEW_COMMENTS=false

        for issue_num in $PREV_ISSUES; do
            # Get current comment count
            CURRENT_INFO=$(gh issue view "$issue_num" --repo "$REPO" --json number,title,state,comments,url 2>/dev/null)
            if [ -n "$CURRENT_INFO" ]; then
                CURRENT_COUNT=$(echo "$CURRENT_INFO" | jq '.comments | length')
                CURRENT_TITLE=$(echo "$CURRENT_INFO" | jq -r '.title')
                CURRENT_STATE=$(echo "$CURRENT_INFO" | jq -r '.state')
                ISSUE_URL=$(echo "$CURRENT_INFO" | jq -r '.url')
                PREV_COUNT=$(echo "$PREV_TRACKING" | jq -r ".issues[] | select(.number == $issue_num) | .comment_count // 0")

                if [ -z "$PREV_COUNT" ]; then
                    PREV_COUNT=0
                fi

                NEW_COMMENTS=$((CURRENT_COUNT - PREV_COUNT))

                if [ "$NEW_COMMENTS" -gt 0 ]; then
                    HAS_NEW_COMMENTS=true
                    REPORT="${REPORT}

• [#${issue_num}](${ISSUE_URL}) ${CURRENT_TITLE}
  📬 +${NEW_COMMENTS}条新评论（总计: ${CURRENT_COUNT}） | 状态: ${CURRENT_STATE}"
                fi
            fi
        done

        if [ "$HAS_NEW_COMMENTS" = false ]; then
            REPORT="${REPORT}

• 暂无新评论"
        fi

        REPORT="${REPORT}

---
"
    fi
fi

# Save current tracking data for tomorrow
echo "Saving issue tracking data for follow-up..."

if [ -z "$ISSUES" ]; then
    TRACKING_DATA='{"date":"'"$DATE"'","issues":[]}'
else
    TRACKING_DATA=$(echo "$ISSUES" | jq '{
  date: "'"$DATE"'",
  issues: [.[] | {
    number: .number,
    title: .title,
    state: .state,
    comment_count: 0,
    url: .url
  }]
}')
fi

echo "$TRACKING_DATA" > "$TRACKING_FILE"
echo "Tracking data saved to $TRACKING_FILE"

# Save report to file
REPORT_FILE="${REPORT_DIR}/report_${DATE}.md"
echo "$REPORT" > "$REPORT_FILE"
echo "Report saved to $REPORT_FILE"

# Follow-up reminder
FOLLOWUP_ISSUES=$(echo "$PREV_TRACKING" | jq -r '[.issues[] | select(.comment_count > 0 or .state == "open")] | "#\(.number) [\(.title)]"' 2>/dev/null || echo "")

if [ -n "$FOLLOWUP_ISSUES" ]; then
    REPORT="${REPORT}

**📌 需要跟进的Issues**

以下Issues需要跟进处理：
${FOLLOWUP_ISSUES}

---

"
fi

# Footer
REPORT="${REPORT}
---
📅 生成时间: $(date +"%Y-%m-%d %H:%M:%S %Z")
🔗 项目: https://github.com/${REPO}
🤖 本报告由 AI 辅助生成，所有链接均可点击跳转
"

# Send report
echo "Sending report to Discord..."
echo "$REPORT" | /root/.nvm/versions/node/v24.13.0/bin/clawdbot message send --channel discord -t "$CHANNEL" -m "$(cat -)"

echo "Done!"
