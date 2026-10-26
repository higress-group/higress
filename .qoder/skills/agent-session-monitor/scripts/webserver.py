#!/usr/bin/env python3
"""
Agent Session Monitor - Web Server
提供浏览器访问的观测界面
"""

import argparse
import json
import sys
from pathlib import Path
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs
from collections import defaultdict
from datetime import datetime, timedelta
import re

# 添加父目录到path以导入cli模块
sys.path.insert(0, str(Path(__file__).parent.parent))

try:
    from scripts.cli import SessionAnalyzer, TOKEN_PRICING
except ImportError:
    # 如果导入失败，定义简单版本
    TOKEN_PRICING = {
        "Qwen3-rerank": {"input": 0.0003, "output": 0.0012},
        "DeepSeek-R1": {"input": 0.004, "output": 0.012, "reasoning": 0.002},
    }


class SessionMonitorHandler(BaseHTTPRequestHandler):
    """HTTP请求处理器"""
    
    def __init__(self, *args, data_dir=None, **kwargs):
        self.data_dir = Path(data_dir) if data_dir else Path("./sessions")
        super().__init__(*args, **kwargs)
    
    def do_GET(self):
        """处理GET请求"""
        parsed_path = urlparse(self.path)
        path = parsed_path.path
        query = parse_qs(parsed_path.query)
        
        if path == '/' or path == '/index.html':
            self.serve_index()
        elif path == '/session':
            session_id = query.get('id', [None])[0]
            if session_id:
                self.serve_session_detail(session_id)
            else:
                self.send_error(400, "Missing session id")
        elif path == '/api/sessions':
            self.serve_api_sessions()
        elif path == '/api/session':
            session_id = query.get('id', [None])[0]
            if session_id:
                self.serve_api_session(session_id)
            else:
                self.send_error(400, "Missing session id")
        elif path == '/api/stats':
            self.serve_api_stats()
        else:
            self.send_error(404, "Not Found")
    
    def serve_index(self):
        """首页 - 总览"""
        html = self.generate_index_html()
        self.send_html(html)
    
    def serve_session_detail(self, session_id: str):
        """Session详情页"""
        html = self.generate_session_html(session_id)
        self.send_html(html)
    
    def serve_api_sessions(self):
        """API: 获取所有session列表"""
        sessions = self.load_all_sessions()
        
        # 简化数据
        data = []
        for session in sessions:
            data.append({
                'session_id': session['session_id'],
                'model': session.get('model', 'unknown'),
                'messages_count': session.get('messages_count', 0),
                'total_tokens': session['total_input_tokens'] + session['total_output_tokens'],
                'updated_at': session.get('updated_at', ''),
                'cost': self.calculate_cost(session)
            })
        
        # 按更新时间降序排序
        data.sort(key=lambda x: x['updated_at'], reverse=True)
        
        self.send_json(data)
    
    def serve_api_session(self, session_id: str):
        """API: 获取指定session的详细数据"""
        session = self.load_session(session_id)
        if session:
            session['cost'] = self.calculate_cost(session)
            self.send_json(session)
        else:
            self.send_error(404, "Session not found")
    
    def serve_api_stats(self):
        """API: 获取统计数据"""
        sessions = self.load_all_sessions()
        
        # 按模型统计
        by_model = defaultdict(lambda: {
            'count': 0,
            'input_tokens': 0,
            'output_tokens': 0,
            'cost': 0.0
        })
        
        # 按日期统计
        by_date = defaultdict(lambda: {
            'count': 0,
            'input_tokens': 0,
            'output_tokens': 0,
            'cost': 0.0,
            'models': set()
        })
        
        total_cost = 0.0
        
        for session in sessions:
            model = session.get('model', 'unknown')
            cost = self.calculate_cost(session)
            total_cost += cost
            
            # 按模型
            by_model[model]['count'] += 1
            by_model[model]['input_tokens'] += session['total_input_tokens']
            by_model[model]['output_tokens'] += session['total_output_tokens']
            by_model[model]['cost'] += cost
            
            # 按日期
            created_at = session.get('created_at', '')
            date_key = created_at[:10] if len(created_at) >= 10 else 'unknown'
            by_date[date_key]['count'] += 1
            by_date[date_key]['input_tokens'] += session['total_input_tokens']
            by_date[date_key]['output_tokens'] += session['total_output_tokens']
            by_date[date_key]['cost'] += cost
            by_date[date_key]['models'].add(model)
        
        # 转换sets为lists
        for date in by_date:
            by_date[date]['models'] = list(by_date[date]['models'])
        
        stats = {
            'total_sessions': len(sessions),
            'total_cost': total_cost,
            'by_model': dict(by_model),
            'by_date': dict(sorted(by_date.items(), reverse=True))
        }
        
        self.send_json(stats)
    
    def load_session(self, session_id: str):
        """加载指定session"""
        session_file = self.data_dir / f"{session_id}.json"
        if session_file.exists():
            with open(session_file, 'r', encoding='utf-8') as f:
                return json.load(f)
        return None
    
    def load_all_sessions(self):
        """加载所有session"""
        sessions = []
        for session_file in self.data_dir.glob("*.json"):
            try:
                with open(session_file, 'r', encoding='utf-8') as f:
                    sessions.append(json.load(f))
            except Exception as e:
                print(f"Warning: Failed to load {session_file}: {e}", file=sys.stderr)
        return sessions
    
    def calculate_cost(self, session: dict) -> float:
        """计算session成本"""
        model = session.get('model', 'unknown')
        pricing = TOKEN_PRICING.get(model, TOKEN_PRICING.get("GPT-4", {"input": 0.003, "output": 0.006}))
        
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
    
    def send_html(self, html: str):
        """发送HTML响应"""
        self.send_response(200)
        self.send_header('Content-type', 'text/html; charset=utf-8')
        self.end_headers()
        self.wfile.write(html.encode('utf-8'))
    
    def send_json(self, data):
        """发送JSON响应"""
        self.send_response(200)
        self.send_header('Content-type', 'application/json; charset=utf-8')
        self.send_header('Access-Control-Allow-Origin', '*')
        self.end_headers()
        self.wfile.write(json.dumps(data, ensure_ascii=False, indent=2).encode('utf-8'))
    
    def generate_index_html(self) -> str:
        """生成首页HTML"""
        return '''<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Agent Session Monitor</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #f5f5f5;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        header {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        h1 { color: #333; margin-bottom: 10px; }
        .subtitle { color: #666; font-size: 14px; }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }
        .stat-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .stat-label { color: #666; font-size: 14px; margin-bottom: 8px; }
        .stat-value { color: #333; font-size: 32px; font-weight: bold; }
        .stat-unit { color: #999; font-size: 16px; margin-left: 4px; }
        
        .section {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        h2 { color: #333; margin-bottom: 20px; font-size: 20px; }
        
        table { width: 100%; border-collapse: collapse; }
        thead { background: #f8f9fa; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #e9ecef; }
        th { font-weight: 600; color: #666; font-size: 14px; }
        td { color: #333; }
        tbody tr:hover { background: #f8f9fa; }
        
        .session-link {
            color: #007bff;
            text-decoration: none;
            font-family: monospace;
            font-size: 13px;
        }
        .session-link:hover { text-decoration: underline; }
        
        .badge {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 500;
        }
        .badge-qwen { background: #e3f2fd; color: #1976d2; }
        .badge-deepseek { background: #f3e5f5; color: #7b1fa2; }
        .badge-gpt { background: #e8f5e9; color: #388e3c; }
        .badge-claude { background: #fff3e0; color: #f57c00; }
        
        .loading { text-align: center; padding: 40px; color: #666; }
        .error { color: #d32f2f; padding: 20px; }
        
        .refresh-btn {
            background: #007bff;
            color: white;
            border: none;
            padding: 10px 20px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
        }
        .refresh-btn:hover { background: #0056b3; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🔍 Agent Session Monitor</h1>
            <p class="subtitle">实时观测Clawdbot对话过程和Token开销</p>
        </header>
        
        <div class="stats-grid" id="stats-grid">
            <div class="stat-card">
                <div class="stat-label">总会话数</div>
                <div class="stat-value">-</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">总Token消耗</div>
                <div class="stat-value">-</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">总成本</div>
                <div class="stat-value">-</div>
            </div>
        </div>
        
        <div class="section">
            <h2>📊 最近会话</h2>
            <button class="refresh-btn" onclick="loadSessions()">🔄 刷新</button>
            <div id="sessions-table">
                <div class="loading">加载中...</div>
            </div>
        </div>
        
        <div class="section">
            <h2>📈 按模型统计</h2>
            <div id="model-stats">
                <div class="loading">加载中...</div>
            </div>
        </div>
    </div>
    
    <script>
        function loadSessions() {
            fetch('/api/sessions')
                .then(r => r.json())
                .then(sessions => {
                    const html = `
                        <table>
                            <thead>
                                <tr>
                                    <th>Session ID</th>
                                    <th>模型</th>
                                    <th>消息数</th>
                                    <th>总Token</th>
                                    <th>成本</th>
                                    <th>更新时间</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${sessions.slice(0, 50).map(s => `
                                    <tr>
                                        <td><a href="/session?id=${encodeURIComponent(s.session_id)}" class="session-link">${s.session_id}</a></td>
                                        <td>${getModelBadge(s.model)}</td>
                                        <td>${s.messages_count}</td>
                                        <td>${s.total_tokens.toLocaleString()}</td>
                                        <td>$${s.cost.toFixed(6)}</td>
                                        <td>${new Date(s.updated_at).toLocaleString()}</td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    `;
                    document.getElementById('sessions-table').innerHTML = html;
                })
                .catch(err => {
                    document.getElementById('sessions-table').innerHTML = `<div class="error">加载失败: ${err.message}</div>`;
                });
        }
        
        function loadStats() {
            fetch('/api/stats')
                .then(r => r.json())
                .then(stats => {
                    // 更新顶部统计卡片
                    const cards = document.querySelectorAll('.stat-card');
                    cards[0].querySelector('.stat-value').textContent = stats.total_sessions;
                    
                    const totalTokens = Object.values(stats.by_model).reduce((sum, m) => sum + m.input_tokens + m.output_tokens, 0);
                    cards[1].querySelector('.stat-value').innerHTML = totalTokens.toLocaleString() + '<span class="stat-unit">tokens</span>';
                    
                    cards[2].querySelector('.stat-value').innerHTML = '$' + stats.total_cost.toFixed(4);
                    
                    // 模型统计表格
                    const modelHtml = `
                        <table>
                            <thead>
                                <tr>
                                    <th>模型</th>
                                    <th>会话数</th>
                                    <th>输入Token</th>
                                    <th>输出Token</th>
                                    <th>成本</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${Object.entries(stats.by_model).map(([model, data]) => `
                                    <tr>
                                        <td>${getModelBadge(model)}</td>
                                        <td>${data.count}</td>
                                        <td>${data.input_tokens.toLocaleString()}</td>
                                        <td>${data.output_tokens.toLocaleString()}</td>
                                        <td>$${data.cost.toFixed(6)}</td>
                                    </tr>
                                `).join('')}
                            </tbody>
                        </table>
                    `;
                    document.getElementById('model-stats').innerHTML = modelHtml;
                })
                .catch(err => {
                    console.error('Failed to load stats:', err);
                });
        }
        
        function getModelBadge(model) {
            let cls = 'badge';
            if (model.includes('Qwen')) cls += ' badge-qwen';
            else if (model.includes('DeepSeek')) cls += ' badge-deepseek';
            else if (model.includes('GPT')) cls += ' badge-gpt';
            else if (model.includes('Claude')) cls += ' badge-claude';
            return `<span class="${cls}">${model}</span>`;
        }
        
        // 初始加载
        loadSessions();
        loadStats();
        
        // 每30秒自动刷新
        setInterval(() => {
            loadSessions();
            loadStats();
        }, 30000);
    </script>
</body>
</html>'''
    
    def generate_session_html(self, session_id: str) -> str:
        """生成Session详情页HTML"""
        session = self.load_session(session_id)
        if not session:
            return f'<html><body><h1>Session not found: {session_id}</h1></body></html>'
        
        cost = self.calculate_cost(session)
        
        # 生成对话轮次HTML
        rounds_html = []
        for r in session.get('rounds', []):
            messages_html = ''
            if r.get('messages'):
                messages_html = '<div class="messages">'
                for msg in r['messages'][-5:]:  # 最多显示5条
                    role = msg.get('role', 'unknown')
                    content = msg.get('content', '')
                    messages_html += f'<div class="message message-{role}"><strong>[{role}]</strong> {self.escape_html(content)}</div>'
                messages_html += '</div>'
            
            tool_calls_html = ''
            if r.get('tool_calls'):
                tool_calls_html = '<div class="tool-calls"><strong>🛠️ Tool Calls:</strong><ul>'
                for tc in r['tool_calls']:
                    func_name = tc.get('function', {}).get('name', 'unknown')
                    tool_calls_html += f'<li>{func_name}()</li>'
                tool_calls_html += '</ul></div>'
            
            # Token详情显示
            token_details_html = ''
            if r.get('input_token_details') or r.get('output_token_details'):
                token_details_html = '<div class="token-details"><strong>📊 Token Details:</strong><ul>'
                if r.get('input_token_details'):
                    token_details_html += f'<li>Input: {r["input_token_details"]}</li>'
                if r.get('output_token_details'):
                    token_details_html += f'<li>Output: {r["output_token_details"]}</li>'
                token_details_html += '</ul></div>'
            
            # Token类型标签
            token_badges = ''
            if r.get('cached_tokens', 0) > 0:
                token_badges += f' <span class="token-badge token-badge-cached">📦 {r["cached_tokens"]:,} cached</span>'
            if r.get('reasoning_tokens', 0) > 0:
                token_badges += f' <span class="token-badge token-badge-reasoning">🧠 {r["reasoning_tokens"]:,} reasoning</span>'
            
            rounds_html.append(f'''
                <div class="round">
                    <div class="round-header">
                        <span class="round-number">Round {r['round']}</span>
                        <span class="round-time">{r['timestamp']}</span>
                        <span class="round-tokens">{r['input_tokens']:,} in → {r['output_tokens']:,} out{token_badges}</span>
                    </div>
                    {messages_html}
                    {f'<div class="question"><strong>❓ Question:</strong> {self.escape_html(r.get("question", ""))}</div>' if r.get('question') else ''}
                    {f'<div class="answer"><strong>✅ Answer:</strong> {self.escape_html(r.get("answer", ""))}</div>' if r.get('answer') else ''}
                    {f'<div class="reasoning"><strong>🧠 Reasoning:</strong> {self.escape_html(r.get("reasoning", ""))}</div>' if r.get('reasoning') else ''}
                    {tool_calls_html}
                    {token_details_html}
                </div>
            ''')
        
        return f'''<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{session_id} - Session Monitor</title>
    <style>
        * {{ margin: 0; padding: 0; box-sizing: border-box; }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #f5f5f5;
            padding: 20px;
        }}
        .container {{ max-width: 1200px; margin: 0 auto; }}
        
        header {{
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }}
        h1 {{ color: #333; margin-bottom: 10px; font-size: 24px; }}
        .back-link {{ color: #007bff; text-decoration: none; margin-bottom: 10px; display: inline-block; }}
        .back-link:hover {{ text-decoration: underline; }}
        
        .info-grid {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-top: 20px;
        }}
        .info-item {{ padding: 10px 0; }}
        .info-label {{ color: #666; font-size: 14px; }}
        .info-value {{ color: #333; font-size: 18px; font-weight: 600; margin-top: 4px; }}
        
        .section {{
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }}
        h2 {{ color: #333; margin-bottom: 20px; font-size: 20px; }}
        
        .round {{
            border-left: 3px solid #007bff;
            padding: 20px;
            margin-bottom: 20px;
            background: #f8f9fa;
            border-radius: 4px;
        }}
        .round-header {{
            display: flex;
            justify-content: space-between;
            margin-bottom: 15px;
            font-size: 14px;
        }}
        .round-number {{ font-weight: 600; color: #007bff; }}
        .round-time {{ color: #666; }}
        .round-tokens {{ color: #333; }}
        
        .messages {{ margin: 15px 0; }}
        .message {{
            padding: 10px;
            margin: 5px 0;
            border-radius: 4px;
            font-size: 14px;
            line-height: 1.6;
        }}
        .message-system {{ background: #fff3cd; }}
        .message-user {{ background: #d1ecf1; }}
        .message-assistant {{ background: #d4edda; }}
        .message-tool {{ background: #e2e3e5; }}
        
        .question, .answer, .reasoning, .tool-calls {{
            margin: 10px 0;
            padding: 10px;
            background: white;
            border-radius: 4px;
            font-size: 14px;
            line-height: 1.6;
        }}
        .question {{ border-left: 3px solid #ffc107; }}
        .answer {{ border-left: 3px solid #28a745; }}
        .reasoning {{ border-left: 3px solid #17a2b8; }}
        .tool-calls {{ border-left: 3px solid #6c757d; }}
        .tool-calls ul {{ margin-left: 20px; margin-top: 5px; }}
        
        .token-details {{
            margin: 10px 0;
            padding: 10px;
            background: white;
            border-radius: 4px;
            font-size: 13px;
            border-left: 3px solid #17a2b8;
        }}
        .token-details ul {{ margin-left: 20px; margin-top: 5px; color: #666; }}
        
        .token-badge {{
            display: inline-block;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 11px;
            margin-left: 5px;
        }}
        .token-badge-cached {{
            background: #d4edda;
            color: #155724;
        }}
        .token-badge-reasoning {{
            background: #cce5ff;
            color: #004085;
        }}
        
        .badge {{
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 500;
            background: #e3f2fd;
            color: #1976d2;
        }}
    </style>
</head>
<body>
    <div class="container">
        <header>
            <a href="/" class="back-link">← 返回列表</a>
            <h1>📊 Session Detail</h1>
            <p style="color: #666; font-family: monospace; font-size: 14px; margin-top: 10px;">{session_id}</p>
            
            <div class="info-grid">
                <div class="info-item">
                    <div class="info-label">模型</div>
                    <div class="info-value"><span class="badge">{session.get('model', 'unknown')}</span></div>
                </div>
                <div class="info-item">
                    <div class="info-label">消息数</div>
                    <div class="info-value">{session.get('messages_count', 0)}</div>
                </div>
                <div class="info-item">
                    <div class="info-label">总Token</div>
                    <div class="info-value">{session['total_input_tokens'] + session['total_output_tokens']:,}</div>
                </div>
                <div class="info-item">
                    <div class="info-label">成本</div>
                    <div class="info-value">${cost:.6f}</div>
                </div>
            </div>
        </header>
        
        <div class="section">
            <h2>💬 对话记录 ({len(session.get('rounds', []))} 轮)</h2>
            {"".join(rounds_html) if rounds_html else '<p style="color: #666;">暂无对话记录</p>'}
        </div>
    </div>
</body>
</html>'''
    
    def escape_html(self, text: str) -> str:
        """转义HTML特殊字符"""
        return (text.replace('&', '&amp;')
                   .replace('<', '&lt;')
                   .replace('>', '&gt;')
                   .replace('"', '&quot;')
                   .replace("'", '&#39;'))
    
    def log_message(self, format, *args):
        """重写日志方法，简化输出"""
        pass  # 不打印每个请求


def create_handler(data_dir):
    """创建带数据目录的处理器"""
    def handler(*args, **kwargs):
        return SessionMonitorHandler(*args, data_dir=data_dir, **kwargs)
    return handler


def main():
    parser = argparse.ArgumentParser(
        description="Agent Session Monitor - Web Server",
        formatter_class=argparse.RawDescriptionHelpFormatter
    )
    
    parser.add_argument(
        '--data-dir',
        default='./sessions',
        help='Session数据目录（默认: ./sessions）'
    )
    
    parser.add_argument(
        '--port',
        type=int,
        default=8888,
        help='HTTP服务器端口（默认: 8888）'
    )
    
    parser.add_argument(
        '--host',
        default='0.0.0.0',
        help='HTTP服务器地址（默认: 0.0.0.0）'
    )
    
    args = parser.parse_args()
    
    # 检查数据目录是否存在
    data_dir = Path(args.data_dir)
    if not data_dir.exists():
        print(f"❌ Error: Data directory not found: {data_dir}")
        print(f"   Please run main.py first to generate session data.")
        sys.exit(1)
    
    # 创建HTTP服务器
    handler_class = create_handler(args.data_dir)
    server = HTTPServer((args.host, args.port), handler_class)
    
    print(f"{'=' * 60}")
    print(f"🌐 Agent Session Monitor - Web Server")
    print(f"{'=' * 60}")
    print()
    print(f"📂 Data directory: {args.data_dir}")
    print(f"🌍 Server address: http://{args.host}:{args.port}")
    print()
    print(f"✅ Server started. Press Ctrl+C to stop.")
    print(f"{'=' * 60}")
    print()
    
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n\n👋 Shutting down server...")
        server.shutdown()


if __name__ == '__main__':
    main()
