#!/usr/bin/env python3
"""
演示如何在Clawdbot中生成Session观测URL
"""

from urllib.parse import quote

def generate_session_url(session_id: str, base_url: str = "http://localhost:8888") -> dict:
    """
    生成session观测URL
    
    Args:
        session_id: 当前会话的session ID
        base_url: Web服务器基础URL
    
    Returns:
        包含各种URL的字典
    """
    # URL编码session_id（处理特殊字符）
    encoded_id = quote(session_id, safe='')
    
    return {
        "session_detail": f"{base_url}/session?id={encoded_id}",
        "api_session": f"{base_url}/api/session?id={encoded_id}",
        "index": f"{base_url}/",
        "api_sessions": f"{base_url}/api/sessions",
        "api_stats": f"{base_url}/api/stats",
    }


def format_response_message(session_id: str, base_url: str = "http://localhost:8888") -> str:
    """
    生成给用户的回复消息
    
    Args:
        session_id: 当前会话的session ID
        base_url: Web服务器基础URL
    
    Returns:
        格式化的回复消息
    """
    urls = generate_session_url(session_id, base_url)
    
    return f"""你的当前会话信息：

📊 **Session ID**: `{session_id}`

🔗 **查看详情**: {urls['session_detail']}

点击链接可以看到：
✅ 完整对话历史（每轮messages）
✅ Token消耗明细（input/output/reasoning）
✅ 工具调用记录
✅ 实时成本统计

**更多链接：**
- 📋 所有会话: {urls['index']}
- 📥 API数据: {urls['api_session']}
- 📊 总体统计: {urls['api_stats']}
"""


# 示例使用
if __name__ == '__main__':
    # 模拟clawdbot的session ID
    demo_session_id = "agent:main:discord:channel:1465367993012981988"
    
    print("=" * 70)
    print("🤖 Clawdbot Session Monitor Demo")
    print("=" * 70)
    print()
    
    # 生成URL
    urls = generate_session_url(demo_session_id)
    
    print("生成的URL：")
    print(f"  Session详情: {urls['session_detail']}")
    print(f"  API数据:     {urls['api_session']}")
    print(f"  总览页面:    {urls['index']}")
    print()
    
    # 生成回复消息
    message = format_response_message(demo_session_id)
    
    print("回复消息模板：")
    print("-" * 70)
    print(message)
    print("-" * 70)
    print()
    
    print("✅ 在Clawdbot中，你可以直接返回上面的消息给用户")
    print()
    
    # 测试特殊字符的session ID
    special_session_id = "agent:test:session/with?special&chars"
    special_urls = generate_session_url(special_session_id)
    
    print("特殊字符处理示例：")
    print(f"  原始ID: {special_session_id}")
    print(f"  URL:    {special_urls['session_detail']}")
    print()
