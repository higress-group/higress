#!/usr/bin/env python3
"""
Agent Session Monitor CLI - 查询和分析agent对话数据
支持：
1. 实时查询指定session的完整llm请求和响应
2. 按模型统计token开销
3. 按日期统计token开销
4. 生成FinOps报表
"""

import argparse
import json
import sys
from collections import defaultdict
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Optional
import re

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


class SessionAnalyzer:
    """Session数据分析器"""
    
    def __init__(self, data_dir: str):
        self.data_dir = Path(data_dir)
        if not self.data_dir.exists():
            raise FileNotFoundError(f"Session data directory not found: {data_dir}")
    
    def load_session(self, session_id: str) -> Optional[dict]:
        """加载指定session的完整数据"""
        session_file = self.data_dir / f"{session_id}.json"
        if not session_file.exists():
            return None
        
        with open(session_file, 'r', encoding='utf-8') as f:
            return json.load(f)
    
    def load_all_sessions(self) -> List[dict]:
        """加载所有session数据"""
        sessions = []
        for session_file in self.data_dir.glob("*.json"):
            try:
                with open(session_file, 'r', encoding='utf-8') as f:
                    session = json.load(f)
                    sessions.append(session)
            except Exception as e:
                print(f"Warning: Failed to load {session_file}: {e}", file=sys.stderr)
        return sessions
    
    def display_session_detail(self, session_id: str, show_messages: bool = True):
        """显示session的详细信息"""
        session = self.load_session(session_id)
        if not session:
            print(f"❌ Session not found: {session_id}")
            return
        
        print(f"\n{'='*70}")
        print(f"📊 Session Detail: {session_id}")
        print(f"{'='*70}\n")
        
        # 基本信息
        print(f"🕐 Created:  {session['created_at']}")
        print(f"🕑 Updated:  {session['updated_at']}")
        print(f"🤖 Model:    {session['model']}")
        print(f"💬 Messages: {session['messages_count']}")
        print()
        
        # Token统计
        print(f"📈 Token Statistics:")
        
        total_input = session['total_input_tokens']
        total_output = session['total_output_tokens']
        total_reasoning = session.get('total_reasoning_tokens', 0)
        total_cached = session.get('total_cached_tokens', 0)
        
        # 区分regular input和cached input
        regular_input = total_input - total_cached
        
        if total_cached > 0:
            print(f"   Input:      {regular_input:>10,} tokens (regular)")
            print(f"   Cached:     {total_cached:>10,} tokens (from cache)")
            print(f"   Total Input:{total_input:>10,} tokens")
        else:
            print(f"   Input:      {total_input:>10,} tokens")
        
        print(f"   Output:     {total_output:>10,} tokens")
        
        if total_reasoning > 0:
            print(f"   Reasoning:  {total_reasoning:>10,} tokens")
        
        # 总计（不重复计算cached）
        total_tokens = total_input + total_output + total_reasoning
        print(f"   ────────────────────────")
        print(f"   Total:      {total_tokens:>10,} tokens")
        print()
        
        # 成本计算
        cost = self._calculate_cost(session)
        print(f"💰 Estimated Cost: ${cost:.8f} USD")
        print()
        
        # 对话轮次
        if show_messages and 'rounds' in session:
            print(f"📝 Conversation Rounds ({len(session['rounds'])}):")
            print(f"{'─'*70}")
            
            for i, round_data in enumerate(session['rounds'], 1):
                timestamp = round_data.get('timestamp', 'N/A')
                input_tokens = round_data.get('input_tokens', 0)
                output_tokens = round_data.get('output_tokens', 0)
                has_tool_calls = round_data.get('has_tool_calls', False)
                response_type = round_data.get('response_type', 'normal')
                
                print(f"\n  Round {i} @ {timestamp}")
                print(f"    Tokens: {input_tokens:,} in → {output_tokens:,} out")
                
                if has_tool_calls:
                    print(f"    🔧 Tool calls: Yes")
                
                if response_type != 'normal':
                    print(f"    Type: {response_type}")
                
                # 显示完整的messages（如果有）
                if 'messages' in round_data:
                    messages = round_data['messages']
                    print(f"    Messages ({len(messages)}):")
                    for msg in messages[-3:]:  # 只显示最后3条
                        role = msg.get('role', 'unknown')
                        content = msg.get('content', '')
                        content_preview = content[:100] + '...' if len(content) > 100 else content
                        print(f"      [{role}] {content_preview}")
                
                # 显示question/answer/reasoning（如果有）
                if 'question' in round_data:
                    q = round_data['question']
                    q_preview = q[:150] + '...' if len(q) > 150 else q
                    print(f"    ❓ Question: {q_preview}")
                
                if 'answer' in round_data:
                    a = round_data['answer']
                    a_preview = a[:150] + '...' if len(a) > 150 else a
                    print(f"    ✅ Answer: {a_preview}")
                
                if 'reasoning' in round_data and round_data['reasoning']:
                    r = round_data['reasoning']
                    r_preview = r[:150] + '...' if len(r) > 150 else r
                    print(f"    🧠 Reasoning: {r_preview}")
                
                if 'tool_calls' in round_data and round_data['tool_calls']:
                    print(f"    🛠️  Tool Calls:")
                    for tool_call in round_data['tool_calls']:
                        func_name = tool_call.get('function', {}).get('name', 'unknown')
                        args = tool_call.get('function', {}).get('arguments', '')
                        print(f"       - {func_name}({args[:80]}...)")
                
                # 显示token details（如果有）
                if round_data.get('input_token_details'):
                    print(f"    📊 Input Token Details: {round_data['input_token_details']}")
                
                if round_data.get('output_token_details'):
                    print(f"    📊 Output Token Details: {round_data['output_token_details']}")
            
            print(f"\n{'─'*70}")
        
        print(f"\n{'='*70}\n")
    
    def _calculate_cost(self, session: dict) -> float:
        """计算session的成本"""
        model = session.get('model', 'unknown')
        pricing = TOKEN_PRICING.get(model, TOKEN_PRICING.get("GPT-4", {}))
        
        input_tokens = session['total_input_tokens']
        output_tokens = session['total_output_tokens']
        reasoning_tokens = session.get('total_reasoning_tokens', 0)
        cached_tokens = session.get('total_cached_tokens', 0)
        
        # 区分regular input和cached input
        regular_input_tokens = input_tokens - cached_tokens
        
        input_cost = regular_input_tokens * pricing.get('input', 0) / 1000000
        output_cost = output_tokens * pricing.get('output', 0) / 1000000
        
        reasoning_cost = 0
        if 'reasoning' in pricing and reasoning_tokens > 0:
            reasoning_cost = reasoning_tokens * pricing['reasoning'] / 1000000
        
        cached_cost = 0
        if 'cached' in pricing and cached_tokens > 0:
            cached_cost = cached_tokens * pricing['cached'] / 1000000
        
        return input_cost + output_cost + reasoning_cost + cached_cost
    
    def stats_by_model(self) -> Dict[str, dict]:
        """按模型统计token开销"""
        sessions = self.load_all_sessions()
        
        stats = defaultdict(lambda: {
            'session_count': 0,
            'total_input': 0,
            'total_output': 0,
            'total_reasoning': 0,
            'total_cost': 0.0
        })
        
        for session in sessions:
            model = session.get('model', 'unknown')
            stats[model]['session_count'] += 1
            stats[model]['total_input'] += session['total_input_tokens']
            stats[model]['total_output'] += session['total_output_tokens']
            stats[model]['total_reasoning'] += session.get('total_reasoning_tokens', 0)
            stats[model]['total_cost'] += self._calculate_cost(session)
        
        return dict(stats)
    
    def stats_by_date(self, days: int = 30) -> Dict[str, dict]:
        """按日期统计token开销（最近N天）"""
        sessions = self.load_all_sessions()
        
        stats = defaultdict(lambda: {
            'session_count': 0,
            'total_input': 0,
            'total_output': 0,
            'total_reasoning': 0,
            'total_cost': 0.0,
            'models': set()
        })
        
        cutoff_date = datetime.now() - timedelta(days=days)
        
        for session in sessions:
            created_at = datetime.fromisoformat(session['created_at'])
            if created_at < cutoff_date:
                continue
            
            date_key = created_at.strftime('%Y-%m-%d')
            stats[date_key]['session_count'] += 1
            stats[date_key]['total_input'] += session['total_input_tokens']
            stats[date_key]['total_output'] += session['total_output_tokens']
            stats[date_key]['total_reasoning'] += session.get('total_reasoning_tokens', 0)
            stats[date_key]['total_cost'] += self._calculate_cost(session)
            stats[date_key]['models'].add(session.get('model', 'unknown'))
        
        # 转换sets为lists以便JSON序列化
        for date_key in stats:
            stats[date_key]['models'] = list(stats[date_key]['models'])
        
        return dict(stats)
    
    def display_model_stats(self):
        """显示按模型的统计"""
        stats = self.stats_by_model()
        
        print(f"\n{'='*80}")
        print(f"📊 Statistics by Model")
        print(f"{'='*80}\n")
        
        print(f"{'Model':<20} {'Sessions':<10} {'Input':<15} {'Output':<15} {'Cost (USD)':<12}")
        print(f"{'─'*80}")
        
        # 按成本降序排列
        sorted_models = sorted(stats.items(), key=lambda x: x[1]['total_cost'], reverse=True)
        
        for model, data in sorted_models:
            print(f"{model:<20} "
                  f"{data['session_count']:<10} "
                  f"{data['total_input']:>12,}  "
                  f"{data['total_output']:>12,}  "
                  f"${data['total_cost']:>10.6f}")
        
        # 总计
        total_sessions = sum(d['session_count'] for d in stats.values())
        total_input = sum(d['total_input'] for d in stats.values())
        total_output = sum(d['total_output'] for d in stats.values())
        total_cost = sum(d['total_cost'] for d in stats.values())
        
        print(f"{'─'*80}")
        print(f"{'TOTAL':<20} "
              f"{total_sessions:<10} "
              f"{total_input:>12,}  "
              f"{total_output:>12,}  "
              f"${total_cost:>10.6f}")
        
        print(f"\n{'='*80}\n")
    
    def display_date_stats(self, days: int = 30):
        """显示按日期的统计"""
        stats = self.stats_by_date(days)
        
        print(f"\n{'='*80}")
        print(f"📊 Statistics by Date (Last {days} days)")
        print(f"{'='*80}\n")
        
        print(f"{'Date':<12} {'Sessions':<10} {'Input':<15} {'Output':<15} {'Cost (USD)':<12} {'Models':<20}")
        print(f"{'─'*80}")
        
        # 按日期升序排列
        sorted_dates = sorted(stats.items())
        
        for date, data in sorted_dates:
            models_str = ', '.join(data['models'][:3])  # 最多显示3个模型
            if len(data['models']) > 3:
                models_str += f" +{len(data['models'])-3}"
            
            print(f"{date:<12} "
                  f"{data['session_count']:<10} "
                  f"{data['total_input']:>12,}  "
                  f"{data['total_output']:>12,}  "
                  f"${data['total_cost']:>10.4f}  "
                  f"{models_str}")
        
        # 总计
        total_sessions = sum(d['session_count'] for d in stats.values())
        total_input = sum(d['total_input'] for d in stats.values())
        total_output = sum(d['total_output'] for d in stats.values())
        total_cost = sum(d['total_cost'] for d in stats.values())
        
        print(f"{'─'*80}")
        print(f"{'TOTAL':<12} "
              f"{total_sessions:<10} "
              f"{total_input:>12,}  "
              f"{total_output:>12,}  "
              f"${total_cost:>10.4f}")
        
        print(f"\n{'='*80}\n")
    
    def list_sessions(self, limit: int = 20, sort_by: str = 'updated'):
        """列出所有session"""
        sessions = self.load_all_sessions()
        
        # 排序
        if sort_by == 'updated':
            sessions.sort(key=lambda s: s.get('updated_at', ''), reverse=True)
        elif sort_by == 'cost':
            sessions.sort(key=lambda s: self._calculate_cost(s), reverse=True)
        elif sort_by == 'tokens':
            sessions.sort(key=lambda s: s['total_input_tokens'] + s['total_output_tokens'], reverse=True)
        
        print(f"\n{'='*100}")
        print(f"📋 Sessions (sorted by {sort_by}, showing {min(limit, len(sessions))} of {len(sessions)})")
        print(f"{'='*100}\n")
        
        print(f"{'Session ID':<30} {'Updated':<20} {'Model':<15} {'Msgs':<6} {'Tokens':<12} {'Cost':<10}")
        print(f"{'─'*100}")
        
        for session in sessions[:limit]:
            session_id = session['session_id'][:28] + '..' if len(session['session_id']) > 30 else session['session_id']
            updated = session.get('updated_at', 'N/A')[:19]
            model = session.get('model', 'unknown')[:13]
            msg_count = session.get('messages_count', 0)
            total_tokens = session['total_input_tokens'] + session['total_output_tokens']
            cost = self._calculate_cost(session)
            
            print(f"{session_id:<30} {updated:<20} {model:<15} {msg_count:<6} {total_tokens:>10,}  ${cost:>8.4f}")
        
        print(f"\n{'='*100}\n")
    
    def export_finops_report(self, output_file: str, format: str = 'json'):
        """导出FinOps报表"""
        model_stats = self.stats_by_model()
        date_stats = self.stats_by_date(30)
        
        report = {
            'generated_at': datetime.now().isoformat(),
            'summary': {
                'total_sessions': sum(d['session_count'] for d in model_stats.values()),
                'total_input_tokens': sum(d['total_input'] for d in model_stats.values()),
                'total_output_tokens': sum(d['total_output'] for d in model_stats.values()),
                'total_cost_usd': sum(d['total_cost'] for d in model_stats.values()),
            },
            'by_model': model_stats,
            'by_date': date_stats,
        }
        
        output_path = Path(output_file)
        
        if format == 'json':
            with open(output_path, 'w', encoding='utf-8') as f:
                json.dump(report, f, ensure_ascii=False, indent=2)
            print(f"✅ FinOps report exported to: {output_path}")
        
        elif format == 'csv':
            import csv
            
            # 按模型导出CSV
            model_csv = output_path.with_suffix('.model.csv')
            with open(model_csv, 'w', newline='', encoding='utf-8') as f:
                writer = csv.writer(f)
                writer.writerow(['Model', 'Sessions', 'Input Tokens', 'Output Tokens', 'Cost (USD)'])
                for model, data in model_stats.items():
                    writer.writerow([
                        model,
                        data['session_count'],
                        data['total_input'],
                        data['total_output'],
                        f"{data['total_cost']:.6f}"
                    ])
            
            # 按日期导出CSV
            date_csv = output_path.with_suffix('.date.csv')
            with open(date_csv, 'w', newline='', encoding='utf-8') as f:
                writer = csv.writer(f)
                writer.writerow(['Date', 'Sessions', 'Input Tokens', 'Output Tokens', 'Cost (USD)', 'Models'])
                for date, data in sorted(date_stats.items()):
                    writer.writerow([
                        date,
                        data['session_count'],
                        data['total_input'],
                        data['total_output'],
                        f"{data['total_cost']:.6f}",
                        ', '.join(data['models'])
                    ])
            
            print(f"✅ FinOps report exported to:")
            print(f"   Model stats: {model_csv}")
            print(f"   Date stats:  {date_csv}")


def main():
    parser = argparse.ArgumentParser(
        description="Agent Session Monitor CLI - 查询和分析agent对话数据",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Commands:
  show <session-id>      显示session的详细信息
  list                   列出所有session
  stats-model            按模型统计token开销
  stats-date             按日期统计token开销（默认30天）
  export                 导出FinOps报表

Examples:
  # 查看特定session的详细对话
  %(prog)s show agent:main:discord:channel:1465367993012981988
  
  # 列出最近20个session（按更新时间）
  %(prog)s list
  
  # 列出token开销最高的10个session
  %(prog)s list --sort-by cost --limit 10
  
  # 按模型统计token开销
  %(prog)s stats-model
  
  # 按日期统计token开销（最近7天）
  %(prog)s stats-date --days 7
  
  # 导出FinOps报表（JSON格式）
  %(prog)s export finops-report.json
  
  # 导出FinOps报表（CSV格式）
  %(prog)s export finops-report --format csv
        """
    )
    
    parser.add_argument(
        'command',
        choices=['show', 'list', 'stats-model', 'stats-date', 'export'],
        help='命令'
    )
    
    parser.add_argument(
        'args',
        nargs='*',
        help='命令参数（例如：session-id或输出文件名）'
    )
    
    parser.add_argument(
        '--data-dir',
        default='./sessions',
        help='Session数据目录（默认: ./sessions）'
    )
    
    parser.add_argument(
        '--limit',
        type=int,
        default=20,
        help='list命令的结果限制（默认: 20）'
    )
    
    parser.add_argument(
        '--sort-by',
        choices=['updated', 'cost', 'tokens'],
        default='updated',
        help='list命令的排序方式（默认: updated）'
    )
    
    parser.add_argument(
        '--days',
        type=int,
        default=30,
        help='stats-date命令的天数（默认: 30）'
    )
    
    parser.add_argument(
        '--format',
        choices=['json', 'csv'],
        default='json',
        help='export命令的输出格式（默认: json）'
    )
    
    parser.add_argument(
        '--no-messages',
        action='store_true',
        help='show命令：不显示对话内容'
    )
    
    args = parser.parse_args()
    
    try:
        analyzer = SessionAnalyzer(args.data_dir)
        
        if args.command == 'show':
            if not args.args:
                parser.error("show命令需要session-id参数")
            session_id = args.args[0]
            analyzer.display_session_detail(session_id, show_messages=not args.no_messages)
        
        elif args.command == 'list':
            analyzer.list_sessions(limit=args.limit, sort_by=args.sort_by)
        
        elif args.command == 'stats-model':
            analyzer.display_model_stats()
        
        elif args.command == 'stats-date':
            analyzer.display_date_stats(days=args.days)
        
        elif args.command == 'export':
            if not args.args:
                parser.error("export命令需要输出文件名参数")
            output_file = args.args[0]
            analyzer.export_finops_report(output_file, format=args.format)
    
    except FileNotFoundError as e:
        print(f"❌ Error: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"❌ Unexpected error: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == '__main__':
    main()
