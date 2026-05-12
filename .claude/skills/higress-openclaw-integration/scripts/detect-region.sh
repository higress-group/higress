#!/bin/bash
# Detect if user is in China region based on timezone
# Returns: "china" or "international"

TIMEZONE=$(cat /etc/timezone 2>/dev/null || timedatectl show --property=Timezone --value 2>/dev/null || echo "Unknown")

# Check if timezone indicates China region (including Hong Kong)
if [[ "$TIMEZONE" == "Asia/Shanghai" ]] || \
   [[ "$TIMEZONE" == "Asia/Hong_Kong" ]] || \
   [[ "$TIMEZONE" == *"China"* ]] || \
   [[ "$TIMEZONE" == *"Beijing"* ]]; then
  echo "china"
else
  echo "international"
fi
