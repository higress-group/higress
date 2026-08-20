#!/usr/bin/env python3
"""
Agent Session Monitor - 实时Agent对话观测程序
监控Higress访问日志，按session聚合对话，追踪token开销
"""

import argparse
import json
import re
import os
import sys
import time
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional

# 使用定时轮询机制，不依赖watchdog

# ============================================================================
# 配置
# ============================================================================

# Token定价（单位：美元/1M tokens）
TOKEN_PRICING = {
    "Qwen": {
        "input": 0.0002,  # $0.2/1M
        "output": 0.0006,
        "cached": 0.0001,  # cached tokens通常是input的50%
    },
    "Qwen3-rerank": {
        "input": 0.0003,
        "output": 0.0012,
        "cached": 0.00015,
    },
    "Qwen-Max": {
        "input": 0.0005,
        "output": 0.002,
        "cached": 0.00025,
    },
    "GPT-4": {
        "input": 0.003,
        "output": 0.006,
        "cached": 0.0015,
    },
    "GPT-4o": {
        "input": 0.0025,
        "output": 0.01,
        "cached": 0.00125,  # GPT-4o prompt caching: 50% discount
    },
    "GPT-4-32k": {
        "input": 0.01,
        "output": 0.03,
        "cached": 0.005,
    },
    "o1": {
        "input": 0.015,
        "output": 0.06,
        "cached": 0.0075,
        "reasoning": 0.06,  # o1 reasoning tokens same as output
    },
    "o1-mini": {
        "input": 0.003,
        "output": 0.012,
        "cached": 0.0015,
        "reasoning": 0.012,
    },
    "Claude": {
        "input": 0.015,
        "output": 0.075,
        "cached": 0.0015,  # Claude prompt caching: 90% discount
    },
    "DeepSeek-R1": {
        "input": 0.004,
        "output": 0.012,
        "reasoning": 0.002,
        "cached": 0.002,
    }
}

DEFAULT_LOG_PATH = "/var/log/higress/access.log"
DEFAULT_OUTPUT_DIR = "./sessions"

# ============================================================================
# Session管理器
# ============================================================================

class SessionManager:
    """管理多个会话的token统计"""
    
    def __init__(self, output_dir: str, load_existing: bool = True):
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.sessions: Dict[str, dict] = {}
        
        # 加载已有的session数据
        if load_existing:
            self._load_existing_sessions()
    
    def _load_existing_sessions(self):
        """加载已有的session数据"""
        loaded_count = 0
        for session_file in self.output_dir.glob("*.json"):
            try:
                with open(session_file, 'r', encoding='utf-8') as f:
                    session = json.load(f)
                    session_id = session.get('session_id')
                    if session_id:
                        self.sessions[session_id] = session
                        loaded_count += 1
            except Exception as e:
                print(f"Warning: Failed to load session {session_file}: {e}", file=sys.stderr)
        
        if loaded_count > 0:
            print(f"📦 Loaded {loaded_count} existing session(s)")
    
    def update_session(self, session_id: str, ai_log: dict) -> dict:
        """更新或创建session"""
        if session_id not in self.sessions:
            self.sessions[session_id] = {
                "session_id": session_id,
                "created_at": datetime.now().isoformat(),
                "updated_at": datetime.now().isoformat(),
                "messages_count": 0,
                "total_input_tokens": 0,
                "total_output_tokens": 0,
                "total_reasoning_tokens": 0,
                "total_cached_tokens": 0,
                "rounds": [],
                "model": ai_log.get("model", "unknown")
            }
        
        session = self.sessions[session_id]
        
        # 更新统计
        model = ai_log.get("model", "unknown")
        session["model"] = model
        session["updated_at"] = datetime.now().isoformat()
        
        # Token统计
        session["total_input_tokens"] += ai_log.get("input_token", 0)
        session["total_output_tokens"] += ai_log.get("output_token", 0)
        
        # 检查reasoning tokens（优先使用ai_log中的reasoning_tokens字段）
        reasoning_tokens = ai_log.get("reasoning_tokens", 0)
        if reasoning_tokens == 0 and "reasoning" in ai_log and ai_log["reasoning"]:
            # 如果没有reasoning_tokens字段，估算reasoning的token数（大致按字符数/4）
            reasoning_text = ai_log["reasoning"]
            reasoning_tokens = len(reasoning_text) // 4
        session["total_reasoning_tokens"] += reasoning_tokens
        
        # 检查cached tokens（prompt caching）
        cached_tokens = ai_log.get("cached_tokens", 0)
        session["total_cached_tokens"] += cached_tokens
        
        # 检查是否有tool_calls（工具调用）
        has_tool_calls = "tool_calls" in ai_log and ai_log["tool_calls"]
        
        # 更新消息数
        session["messages_count"] += 1
        
        # 解析token details（如果有）
        input_token_details = {}
        output_token_details = {}
        
        if "input_token_details" in ai_log:
            try:
                # input_token_details可能是字符串或字典
                details = ai_log["input_token_details"]
                if isinstance(details, str):
                    import json
                    input_token_details = json.loads(details)
                else:
                    input_token_details = details
            except (json.JSONDecodeError, TypeError):
                pass
        
        if "output_token_details" in ai_log:
            try:
                # output_token_details可能是字符串或字典
                details = ai_log["output_token_details"]
                if isinstance(details, str):
                    import json
                    output_token_details = json.loads(details)
                else:
                    output_token_details = details
            except (json.JSONDecodeError, TypeError):
                pass
        
        # 添加轮次记录（包含完整的llm请求和响应信息）
        round_data = {
            "round": session["messages_count"],
            "timestamp": datetime.now().isoformat(),
            "input_tokens": ai_log.get("input_token", 0),
            "output_tokens": ai_log.get("output_token", 0),
            "reasoning_tokens": reasoning_tokens,
            "cached_tokens": cached_tokens,
            "model": model,
            "has_tool_calls": has_tool_calls,
            "response_type": ai_log.get("response_type", "normal"),
            # 完整的对话信息
            "messages": ai_log.get("messages", []),
            "question": ai_log.get("question", ""),
            "answer": ai_log.get("answer", ""),
            "reasoning": ai_log.get("reasoning", ""),
            "tool_calls": ai_log.get("tool_calls", []),
            # Token详情
            "input_token_details": input_token_details,
            "output_token_details": output_token_details,
        }
        session["rounds"].append(round_data)
        
        # 保存到文件
        self._save_session(session)
        
        return session
    
    def _save_session(self, session: dict):
        """保存session数据到文件"""
        session_file = self.output_dir / f"{session['session_id']}.json"
        with open(session_file, 'w', encoding='utf-8') as f:
            json.dump(session, f, ensure_ascii=False, indent=2)
    
    def get_all_sessions(self) -> List[dict]:
        """获取所有session"""
        return list(self.sessions.values())
    
    def get_session(self, session_id: str) -> Optional[dict]:
        """获取指定session"""
        return self.sessions.get(session_id)
    
    def get_summary(self) -> dict:
        """获取总体统计"""
        total_input = sum(s["total_input_tokens"] for s in self.sessions.values())
        total_output = sum(s["total_output_tokens"] for s in self.sessions.values())
        total_reasoning = sum(s.get("total_reasoning_tokens", 0) for s in self.sessions.values())
        total_cached = sum(s.get("total_cached_tokens", 0) for s in self.sessions.values())
        
        # 计算成本
        total_cost = 0
        for session in self.sessions.values():
            model = session.get("model", "unknown")
            input_tokens = session["total_input_tokens"]
            output_tokens = session["total_output_tokens"]
            reasoning_tokens = session.get("total_reasoning_tokens", 0)
            cached_tokens = session.get("total_cached_tokens", 0)
            
            pricing = TOKEN_PRICING.get(model, TOKEN_PRICING.get("GPT-4", {}))
            
            # 基础成本计算
            # 注意：cached_tokens已经包含在input_tokens中，需要分开计算
            regular_input_tokens = input_tokens - cached_tokens
            input_cost = regular_input_tokens * pricing.get("input", 0) / 1000000
            output_cost = output_tokens * pricing.get("output", 0) / 1000000
            
            # reasoning成本
            reasoning_cost = 0
            if "reasoning" in pricing and reasoning_tokens > 0:
                reasoning_cost = reasoning_tokens * pricing["reasoning"] / 1000000
            
            # cached成本（通常比input便宜）
            cached_cost = 0
            if "cached" in pricing and cached_tokens > 0:
                cached_cost = cached_tokens * pricing["cached"] / 1000000
            
            total_cost += input_cost + output_cost + reasoning_cost + cached_cost
        
        return {
            "total_sessions": len(self.sessions),
            "total_input_tokens": total_input,
            "total_output_tokens": total_output,
            "total_reasoning_tokens": total_reasoning,
            "total_cached_tokens": total_cached,
            "total_tokens": total_input + total_output + total_reasoning + total_cached,
            "total_cost_usd": round(total_cost, 4),
            "active_session_ids": list(self.sessions.keys())
        }


# ============================================================================
# 日志解析器
# ============================================================================

class LogParser:
    """解析Higress访问日志，提取ai_log，支持日志轮转"""
    
    def __init__(self, state_file: str = None):
        self.state_file = Path(state_file) if state_file else None
        self.file_offsets = {}  # {文件路径: 已读取的字节偏移}
        self._load_state()
    
    def _load_state(self):
        """加载上次的读取状态"""
        if self.state_file and self.state_file.exists():
            try:
                with open(self.state_file, 'r') as f:
                    self.file_offsets = json.load(f)
            except Exception as e:
                print(f"Warning: Failed to load state file: {e}", file=sys.stderr)
    
    def _save_state(self):
        """保存当前的读取状态"""
        if self.state_file:
            try:
                self.state_file.parent.mkdir(parents=True, exist_ok=True)
                with open(self.state_file, 'w') as f:
                    json.dump(self.file_offsets, f, indent=2)
            except Exception as e:
                print(f"Warning: Failed to save state file: {e}", file=sys.stderr)
    
    def parse_log_line(self, line: str) -> Optional[dict]:
        """解析单行日志，提取ai_log JSON"""
        try:
            # 直接解析整个日志行为JSON
            log_obj = json.loads(line.strip())
            
            # 获取ai_log字段（这是一个JSON字符串）
            if 'ai_log' in log_obj:
                ai_log_str = log_obj['ai_log']
                
                # 解析内层JSON
                ai_log = json.loads(ai_log_str)
                return ai_log
        except (json.JSONDecodeError, ValueError, KeyError):
            # 静默忽略非JSON行或缺少ai_log字段的行
            pass
        
        return None
    
    def parse_rotated_logs(self, log_pattern: str, session_manager) -> None:
        """解析日志文件及其轮转文件
        
        Args:
            log_pattern: 日志文件路径，如 /var/log/proxy/access.log
            session_manager: Session管理器
        """
        base_path = Path(log_pattern)
        
        # 自动扫描所有轮转的日志文件（从旧到新）
        log_files = []
        
        # 自动扫描轮转文件（最多扫描到 .100，超过这个数量的日志应该很少见）
        for i in range(100, 0, -1):
            rotated_path = Path(f"{log_pattern}.{i}")
            if rotated_path.exists():
                log_files.append(str(rotated_path))
        
        # 添加当前日志文件
        if base_path.exists():
            log_files.append(str(base_path))
        
        if not log_files:
            print(f"❌ No log files found for pattern: {log_pattern}")
            return
        
        print(f"📂 Found {len(log_files)} log file(s):")
        for f in log_files:
            print(f"   - {f}")
        print()
        
        # 按顺序解析每个文件（从旧到新）
        for log_file in log_files:
            self._parse_file_incremental(log_file, session_manager)
        
        # 保存状态
        self._save_state()
    
    def _parse_file_incremental(self, file_path: str, session_manager) -> None:
        """增量解析单个日志文件"""
        try:
            file_stat = os.stat(file_path)
            file_size = file_stat.st_size
            file_inode = file_stat.st_ino
            
            # 使用inode作为主键
            inode_key = str(file_inode)
            last_offset = self.file_offsets.get(inode_key, 0)
            
            # 如果文件变小了，说明是新文件（被truncate或新创建），从头开始读
            if file_size < last_offset:
                print(f"   📝 File truncated or recreated, reading from start: {file_path}")
                last_offset = 0
            
            # 如果offset相同，说明没有新内容
            if file_size == last_offset:
                print(f"   ⏭️  No new content in: {file_path} (inode:{inode_key})")
                return
            
            print(f"   📖 Reading {file_path} from offset {last_offset} to {file_size} (inode:{inode_key})")
            
            with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                f.seek(last_offset)
                lines_processed = 0
                
                for line in f:
                    ai_log = self.parse_log_line(line)
                    if ai_log:
                        session_id = ai_log.get("session_id", "default")
                        session_manager.update_session(session_id, ai_log)
                        lines_processed += 1
                        
                        # 每处理1000行打印一次进度
                        if lines_processed % 1000 == 0:
                            print(f"      Processed {lines_processed} lines, {len(session_manager.sessions)} sessions")
                
                # 更新offset（使用inode作为key）
                current_offset = f.tell()
                self.file_offsets[inode_key] = current_offset
                
                print(f"   ✅ Processed {lines_processed} new lines from {file_path}")
                
        except FileNotFoundError:
            print(f"   ❌ File not found: {file_path}")
        except Exception as e:
            print(f"   ❌ Error parsing {file_path}: {e}")


# ============================================================================
# 实时显示器
# ============================================================================

class RealtimeMonitor:
    """实时监控显示和交互（定时轮询模式）"""
    
    def __init__(self, session_manager: SessionManager, log_parser=None, log_path: str = None, refresh_interval: int = 1):
        self.session_manager = session_manager
        self.log_parser = log_parser
        self.log_path = log_path
        self.refresh_interval = refresh_interval
        self.running = True
        self.last_poll_time = 0
    
    def start(self):
        """启动实时监控（定时轮询日志文件）"""
        print(f"\n{'=' * 50}")
        print(f"🔍 Agent Session Monitor - Real-time View")
        print(f"{'=' * 50}")
        print()
        print("Press Ctrl+C to stop...")
        print()
        
        try:
            while self.running:
                # 定时轮询日志文件（检查新增内容和轮转）
                current_time = time.time()
                if self.log_parser and self.log_path and (current_time - self.last_poll_time >= self.refresh_interval):
                    self.log_parser.parse_rotated_logs(self.log_path, self.session_manager)
                    self.last_poll_time = current_time
                
                # 显示状态
                self._display_status()
                time.sleep(self.refresh_interval)
        except KeyboardInterrupt:
            print("\n\n👋 Stopping monitor...")
            self.running = False
            self._display_summary()
    
    def _display_status(self):
        """显示当前状态"""
        summary = self.session_manager.get_summary()
        
        # 清屏
        os.system('clear' if os.name == 'posix' else 'cls')
        
        print(f"{'=' * 50}")
        print(f"🔍 Session Monitor - Active")
        print(f"{'=' * 50}")
        print()
        print(f"📊 Active Sessions: {summary['total_sessions']}")
        print()
        
        # 显示活跃session的token统计
        if summary['active_session_ids']:
            print("┌──────────────────────────┬─────────┬──────────┬───────────┐")
            print("│ Session ID               │ Msgs    │ Input    │ Output    │")
            print("├──────────────────────────┼─────────┼──────────┼───────────┤")
            
            for session_id in summary['active_session_ids'][:10]:  # 最多显示10个
                session = self.session_manager.get_session(session_id)
                if session:
                    sid = session_id[:24] if len(session_id) > 24 else session_id
                    print(f"│ {sid:<24} │ {session['messages_count']:>7} │ {session['total_input_tokens']:>8,} │ {session['total_output_tokens']:>9,} │")
            
            print("└──────────────────────────┴─────────┴──────────┴───────────┘")
        
        print()
        print(f"📈 Token Statistics")
        print(f"   Total Input:   {summary['total_input_tokens']:,} tokens")
        print(f"   Total Output:  {summary['total_output_tokens']:,} tokens")
        if summary['total_reasoning_tokens'] > 0:
            print(f"   Total Reasoning: {summary['total_reasoning_tokens']:,} tokens")
        print(f"   Total Cached:   {summary['total_cached_tokens']:,} tokens")
        print(f"   Total Cost:     ${summary['total_cost_usd']:.4f}")
    
    def _display_summary(self):
        """显示最终汇总"""
        summary = self.session_manager.get_summary()
        
        print()
        print(f"{'=' * 50}")
        print(f"📊 Session Monitor - Summary")
        print(f"{'=' * 50}")
        print()
        print(f"📈 Final Statistics")
        print(f"   Total Sessions: {summary['total_sessions']}")
        print(f"   Total Input:   {summary['total_input_tokens']:,} tokens")
        print(f"   Total Output:  {summary['total_output_tokens']:,} tokens")
        if summary['total_reasoning_tokens'] > 0:
            print(f"   Total Reasoning: {summary['total_reasoning_tokens']:,} tokens")
        print(f"   Total Cached:   {summary['total_cached_tokens']:,} tokens")
        print(f"   Total Tokens:   {summary['total_tokens']:,} tokens")
        print(f"   Total Cost:     ${summary['total_cost_usd']:.4f}")
        print(f"{'=' * 50}")
        print()


# ============================================================================
# 主程序
# ============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Agent Session Monitor - 实时监控多轮Agent对话的token开销",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 监控默认日志
  %(prog)s
  
  # 监控指定日志文件
  %(prog)s --log-path /var/log/higress/access.log
  
  # 设置预算为500K tokens
  %(prog)s --budget 500000
  
  # 监控特定session
  %(prog)s --session-key agent:main:discord:channel:1465367993012981988
        """,
        allow_abbrev=False
    )
    
    parser.add_argument(
        '--log-path',
        default=DEFAULT_LOG_PATH,
        help=f'Higress访问日志文件路径（默认: {DEFAULT_LOG_PATH}）'
    )
    
    parser.add_argument(
        '--output-dir',
        default=DEFAULT_OUTPUT_DIR,
        help=f'Session数据存储目录（默认: {DEFAULT_OUTPUT_DIR}）'
    )
    
    parser.add_argument(
        '--session-key',
        help='只监控包含指定session key的日志'
    )
    
    parser.add_argument(
        '--refresh-interval',
        type=int,
        default=1,
        help=f'实时监控刷新间隔（秒，默认: 1）'
    )
    
    parser.add_argument(
        '--state-file',
        help='状态文件路径，用于记录已读取的offset（默认: <output-dir>/.state.json）'
    )
    
    args = parser.parse_args()
    
    # 初始化组件
    session_manager = SessionManager(output_dir=args.output_dir)
    
    # 状态文件路径
    state_file = args.state_file or str(Path(args.output_dir) / '.state.json')
    
    log_parser = LogParser(state_file=state_file)
    
    print(f"{'=' * 60}")
    print(f"🔍 Agent Session Monitor")
    print(f"{'=' * 60}")
    print()
    print(f"📂 Log path: {args.log_path}")
    print(f"📁 Output dir: {args.output_dir}")
    if args.session_key:
        print(f"🔑 Session key filter: {args.session_key}")
    print(f"{'=' * 60}")
    print()
    
    # 模式选择：实时监控或单次解析
    if len(sys.argv) == 1:
        # 默认模式：实时监控（定时轮询）
        print("📺 Mode: Real-time monitoring (polling mode with log rotation support)")
        print(f"   Refresh interval: {args.refresh_interval} second(s)")
        print()
        
        # 首次解析现有日志文件（包括轮转的文件）
        log_parser.parse_rotated_logs(args.log_path, session_manager)
        
        # 启动实时监控（定时轮询模式）
        monitor = RealtimeMonitor(
            session_manager, 
            log_parser=log_parser,
            log_path=args.log_path,
            refresh_interval=args.refresh_interval
        )
        monitor.start()
        
    else:
        # 单次解析模式
        print("📊 Mode: One-time log parsing (with log rotation support)")
        print()
        log_parser.parse_rotated_logs(args.log_path, session_manager)
        
        # 显示汇总
        summary = session_manager.get_summary()
        print(f"\n{'=' * 50}")
        print(f"📊 Session Summary")
        print(f"{'=' * 50}")
        print()
        print(f"📈 Final Statistics")
        print(f"   Total Sessions: {summary['total_sessions']}")
        print(f"   Total Input:   {summary['total_input_tokens']:,} tokens")
        print(f"   Total Output:  {summary['total_output_tokens']:,} tokens")
        if summary['total_reasoning_tokens'] > 0:
            print(f"   Total Reasoning: {summary['total_reasoning_tokens']:,} tokens")
        print(f"   Total Cached:   {summary['total_cached_tokens']:,} tokens")
        print(f"   Total Tokens:   {summary['total_tokens']:,} tokens")
        print(f"   Total Cost:     ${summary['total_cost_usd']:.4f}")
        print(f"{'=' * 50}")
        print()
        print(f"💾 Session data saved to: {args.output_dir}/")
        print(f"   Run with --output-dir to specify custom directory")


if __name__ == '__main__':
    main()
