#!/bin/bash
# Agent Session Monitor - Demo for PR #3424 token details

set -e

SKILL_DIR="$(dirname "$(dirname "$(realpath "$0")")")"
EXAMPLE_DIR="$SKILL_DIR/example"
LOG_FILE="$EXAMPLE_DIR/test_access_v2.log"
OUTPUT_DIR="$EXAMPLE_DIR/sessions_v2"

echo "========================================"
echo "Agent Session Monitor - Token Details Demo"
echo "========================================"
echo ""

# 清理旧数据
if [ -d "$OUTPUT_DIR" ]; then
    echo "🧹 Cleaning up old session data..."
    rm -rf "$OUTPUT_DIR"
fi

echo "📂 Log file: $LOG_FILE"
echo "📁 Output dir: $OUTPUT_DIR"
echo ""

# 步骤1：解析日志文件
echo "========================================"
echo "步骤1：解析日志文件（包含token details）"
echo "========================================"
python3 "$SKILL_DIR/main.py" \
    --log-path "$LOG_FILE" \
    --output-dir "$OUTPUT_DIR"

echo ""
echo "✅ 日志解析完成！Session数据已保存到: $OUTPUT_DIR"
echo ""

# 步骤2：查看使用prompt caching的session（gpt-4o）
echo "========================================"
echo "步骤2：查看GPT-4o session（包含cached tokens）"
echo "========================================"
python3 "$SKILL_DIR/scripts/cli.py" show "agent:main:discord:1465367993012981988" \
    --data-dir "$OUTPUT_DIR"

# 步骤3：查看使用reasoning的session（o1）
echo "========================================"
echo "步骤3：查看o1 session（包含reasoning tokens）"
echo "========================================"
python3 "$SKILL_DIR/scripts/cli.py" show "agent:main:discord:9999999999999999999" \
    --data-dir "$OUTPUT_DIR"

# 步骤4：按模型统计
echo "========================================"
echo "步骤4：按模型统计（包含新token类型）"
echo "========================================"
python3 "$SKILL_DIR/scripts/cli.py" stats-model \
    --data-dir "$OUTPUT_DIR"

echo ""
echo "========================================"
echo "✅ Demo完成！"
echo "========================================"
echo ""
echo "💡 新功能说明："
echo "  ✅ cached_tokens - 缓存命中的token数（prompt caching）"
echo "  ✅ reasoning_tokens - 推理token数（o1等模型）"
echo "  ✅ input_token_details - 完整输入token详情（JSON）"
echo "  ✅ output_token_details - 完整输出token详情（JSON）"
echo ""
echo "💰 成本计算已优化："
echo "  - cached tokens通常比regular input便宜（50-90%折扣）"
echo "  - reasoning tokens单独计费（o1系列）"
echo ""
echo "🌐 启动Web界面查看："
echo "  python3 $SKILL_DIR/scripts/webserver.py --data-dir $OUTPUT_DIR --port 8889"
echo "  然后访问: http://localhost:8889"
