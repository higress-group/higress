#!/bin/bash
# 测试日志轮转功能

set -e

SKILL_DIR="$(dirname "$(dirname "$(realpath "$0")")")"
EXAMPLE_DIR="$SKILL_DIR/example"
TEST_DIR="$EXAMPLE_DIR/rotation_test"
LOG_FILE="$TEST_DIR/access.log"
OUTPUT_DIR="$TEST_DIR/sessions"

echo "========================================"
echo "Log Rotation Test"
echo "========================================"
echo ""

# 清理旧测试数据
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"

echo "📁 Test directory: $TEST_DIR"
echo ""

# 模拟日志轮转场景
echo "========================================"
echo "步骤1：创建初始日志文件"
echo "========================================"

# 创建第一批日志（10条）
for i in {1..10}; do
    echo "{\"timestamp\":\"2026-02-01T10:0${i}:00Z\",\"ai_log\":\"{\\\"session_id\\\":\\\"session_001\\\",\\\"model\\\":\\\"gpt-4o\\\",\\\"input_token\\\":$((100+i)),\\\"output_token\\\":$((50+i)),\\\"cached_tokens\\\":$((30+i))}\"}" >> "$LOG_FILE"
done

echo "✅ Created $LOG_FILE with 10 lines"
echo ""

# 首次解析
echo "========================================"
echo "步骤2：首次解析（应该处理10条记录）"
echo "========================================"
python3 "$SKILL_DIR/main.py" \
    --log-path "$LOG_FILE" \
    --output-dir "$OUTPUT_DIR" \
    

echo ""

# 检查session数据
echo "Session数据："
cat "$OUTPUT_DIR/session_001.json" | python3 -c "import sys, json; d=json.load(sys.stdin); print(f\"  Messages: {d['messages_count']}, Total Input: {d['total_input_tokens']}\")"
echo ""

# 模拟日志轮转
echo "========================================"
echo "步骤3：模拟日志轮转"
echo "========================================"
mv "$LOG_FILE" "$LOG_FILE.1"
echo "✅ Rotated: access.log -> access.log.1"
echo ""

# 创建新的日志文件（5条新记录）
for i in {11..15}; do
    echo "{\"timestamp\":\"2026-02-01T10:${i}:00Z\",\"ai_log\":\"{\\\"session_id\\\":\\\"session_001\\\",\\\"model\\\":\\\"gpt-4o\\\",\\\"input_token\\\":$((100+i)),\\\"output_token\\\":$((50+i)),\\\"cached_tokens\\\":$((30+i))}\"}" >> "$LOG_FILE"
done

echo "✅ Created new $LOG_FILE with 5 lines"
echo ""

# 再次解析（应该只处理新的5条）
echo "========================================"
echo "步骤4：再次解析（应该只处理新的5条）"
echo "========================================"
python3 "$SKILL_DIR/main.py" \
    --log-path "$LOG_FILE" \
    --output-dir "$OUTPUT_DIR" \
    

echo ""

# 检查session数据
echo "Session数据："
cat "$OUTPUT_DIR/session_001.json" | python3 -c "import sys, json; d=json.load(sys.stdin); print(f\"  Messages: {d['messages_count']}, Total Input: {d['total_input_tokens']} (应该是15条记录)\")"
echo ""

# 再次轮转
echo "========================================"
echo "步骤5：再次轮转"
echo "========================================"
mv "$LOG_FILE.1" "$LOG_FILE.2"
mv "$LOG_FILE" "$LOG_FILE.1"
echo "✅ Rotated: access.log -> access.log.1"
echo "✅ Rotated: access.log.1 -> access.log.2"
echo ""

# 创建新的日志文件（3条新记录）
for i in {16..18}; do
    echo "{\"timestamp\":\"2026-02-01T10:${i}:00Z\",\"ai_log\":\"{\\\"session_id\\\":\\\"session_001\\\",\\\"model\\\":\\\"gpt-4o\\\",\\\"input_token\\\":$((100+i)),\\\"output_token\\\":$((50+i)),\\\"cached_tokens\\\":$((30+i))}\"}" >> "$LOG_FILE"
done

echo "✅ Created new $LOG_FILE with 3 lines"
echo ""

# 再次解析（应该只处理新的3条）
echo "========================================"
echo "步骤6：再次解析（应该只处理新的3条）"
echo "========================================"
python3 "$SKILL_DIR/main.py" \
    --log-path "$LOG_FILE" \
    --output-dir "$OUTPUT_DIR" \
    

echo ""

# 检查session数据
echo "Session数据："
cat "$OUTPUT_DIR/session_001.json" | python3 -c "import sys, json; d=json.load(sys.stdin); print(f\"  Messages: {d['messages_count']}, Total Input: {d['total_input_tokens']} (应该是18条记录)\")"
echo ""

# 检查状态文件
echo "========================================"
echo "步骤7：查看状态文件"
echo "========================================"
echo "状态文件内容："
cat "$OUTPUT_DIR/.state.json" | python3 -m json.tool | head -20
echo ""

echo "========================================"
echo "✅ 测试完成！"
echo "========================================"
echo ""
echo "💡 验证要点："
echo "  1. 首次解析处理了10条记录"
echo "  2. 轮转后只处理新增的5条记录（总计15条）"
echo "  3. 再次轮转后只处理新增的3条记录（总计18条）"
echo "  4. 状态文件记录了每个文件的inode和offset"
echo ""
echo "📂 测试数据保存在: $TEST_DIR/"
