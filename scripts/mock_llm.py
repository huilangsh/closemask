#!/usr/bin/env python3
"""Mock LLM - 模拟 OpenAI 兼容的 LLM 服务

支持功能：
- 非流式和流式 (SSE) 响应
- 工具调用 (tool_calls) - 当请求携带 tools 参数时自动触发
- 多轮对话 - 收到 tool 角色消息时返回总结文本
- 智能工具选择 - 根据用户消息内容从可用工具中选择最合适的
"""

import time
import json
import re
import uuid
from flask import Flask, request, Response, stream_with_context, jsonify
from datetime import datetime

app = Flask(__name__)

MODEL_NAME = "mock-llm"

# 存储最后一个请求用于测试
last_request_store = {}


def generate_chat_id():
    return f"chatcmpl-{datetime.now().strftime('%Y%m%d%H%M%S')}-{uuid.uuid4().hex[:8]}"


def create_chunk(choice_data, finish_reason=None):
    """创建 SSE 数据块"""
    chunk = {
        "id": generate_chat_id(),
        "object": "chat.completion.chunk",
        "created": int(datetime.now().timestamp()),
        "model": MODEL_NAME,
        "choices": [choice_data]
    }
    if finish_reason:
        chunk["choices"][0]["finish_reason"] = finish_reason
    return f"data: {json.dumps(chunk, ensure_ascii=False)}\n\n"


def make_usage(prompt_tokens, completion_tokens):
    """构造 usage 字段"""
    return {
        "prompt_tokens": prompt_tokens,
        "completion_tokens": completion_tokens,
        "total_tokens": prompt_tokens + completion_tokens
    }


# ============ 工具调用参数提取逻辑 ============

def extract_phone(text):
    """从文本中提取手机号"""
    m = re.search(r'1[3-9]\d{9}', text)
    return m.group() if m else None


def extract_email(text):
    """从文本中提取邮箱"""
    m = re.search(r'[\w.+-]+@[\w-]+\.[\w.]+', text)
    return m.group() if m else None


def extract_id_card(text):
    """从文本中提取身份证号"""
    m = re.search(r'[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]', text)
    return m.group() if m else None


def extract_bank_card(text):
    """从文本中提取银行卡号"""
    m = re.search(r'\d{16,19}', text)
    return m.group() if m else None


def extract_access_token(text):
    """从文本中提取 JWT accessToken"""
    m = re.search(r'eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+', text)
    return m.group() if m else None


def extract_verification_code(text):
    """从文本中提取验证码"""
    m = re.search(r'验证码[是为：:\s]*(\d{4,8})', text)
    return m.group(1) if m else None


def extract_city(text):
    """从文本中提取城市名"""
    cities = ["北京", "上海", "广州", "深圳", "杭州", "成都", "武汉", "南京", "重庆", "天津",
              "苏州", "西安", "长沙", "沈阳", "青岛", "郑州", "大连", "东莞", "宁波", "厦门"]
    for city in cities:
        if city in text:
            return city
    return None


def extract_amount(text):
    """从文本中提取转账金额"""
    m = re.search(r'(\d+(?:\.\d+)?)\s*元', text)
    return float(m.group(1)) if m else None


def extract_account(text):
    """从文本中提取对方账号"""
    m = re.search(r'账号[是为：:\s]*(\d{16,19})', text)
    return m.group(1) if m else None


def extract_to_phone(text):
    """从文本中提取对方手机号"""
    # 查找"给XXX"后面的手机号，或"他/她的手机号"
    m = re.search(r'(?:给|对方的?)手机号[是为：:\s]*(1[3-9]\d{9})', text)
    if m:
        return m.group(1)
    # 查找转账给某人后面的手机号
    m = re.search(r'转账给\w+[，,]?\s*(?:他|她|对方)?(?:的)?手机号[是为：:\s]*(1[3-9]\d{9})', text)
    return m.group(1) if m else None


def extract_expression(text):
    """从文本中提取数学表达式"""
    m = re.search(r'(\d+[\s]*[+\-*/][\s]*\d+)', text)
    return m.group(1) if m else None


def extract_query(text):
    """从文本中提取搜索关键词"""
    return text.strip()[:50]


# ============ 工具参数构建 ============

def build_tool_args(tool_name, text):
    """根据工具名和用户文本，智能构建工具调用参数"""
    args = {}

    if tool_name == "get_user_info":
        phone = extract_phone(text)
        if phone:
            args["phone"] = phone
        email = extract_email(text)
        if email:
            args["email"] = email
        id_card = extract_id_card(text)
        if id_card:
            args["id_card"] = id_card
        token = extract_access_token(text)
        if token:
            args["access_token"] = token

    elif tool_name == "check_user_status":
        token = extract_access_token(text)
        if token:
            args["access_token"] = token

    elif tool_name == "get_weather":
        city = extract_city(text)
        if city:
            args["city"] = city
        else:
            args["city"] = "北京"

    elif tool_name == "get_balance":
        id_card = extract_id_card(text)
        if id_card:
            args["id_card"] = id_card
        phone = extract_phone(text)
        if phone:
            args["phone"] = phone

    elif tool_name == "transfer_money":
        # 需要从文本提取收款方信息
        # 尝试从文本中找出对方手机号和账号
        all_phones = re.findall(r'1[3-9]\d{9}', text)
        from_phone = extract_phone(text)
        if from_phone and len(all_phones) > 1:
            args["from_phone"] = from_phone
            args["to_phone"] = all_phones[1]
        elif len(all_phones) >= 2:
            args["from_phone"] = all_phones[0]
            args["to_phone"] = all_phones[1]
        elif from_phone:
            args["from_phone"] = from_phone

        account = extract_account(text)
        if account:
            args["to_account"] = account

        amount = extract_amount(text)
        if amount:
            args["amount"] = amount

    elif tool_name == "get_transactions":
        phone = extract_phone(text)
        if phone:
            args["phone"] = phone

    elif tool_name == "change_password":
        phone = extract_phone(text)
        if phone:
            args["phone"] = phone
        code = extract_verification_code(text)
        if code:
            args["verification_code"] = code

    elif tool_name == "search":
        args["query"] = extract_query(text)

    elif tool_name == "calculate":
        expr = extract_expression(text)
        if expr:
            args["expression"] = expr

    return args


# ============ 工具选择逻辑 ============

# 关键词到工具名的映射
TOOL_KEYWORDS = {
    "get_user_info": ["用户信息", "查询用户", "user_info", "get_user"],
    "check_user_status": ["用户状态", "检查状态", "check_status"],
    "get_weather": ["天气", "weather", "气温", "温度"],
    "get_balance": ["余额", "balance", "账户余额"],
    "transfer_money": ["转账", "transfer", "汇款", "打钱"],
    "get_transactions": ["交易记录", "交易", "账单", "流水", "transactions"],
    "change_password": ["修改密码", "改密码", "重置密码", "change_password"],
    "search": ["搜索", "查询", "search", "查一下"],
    "calculate": ["计算", "calculate", "算一下"],
}


def select_tool(tools, user_text):
    """从可用工具列表中选择最匹配的工具

    优先级：
    1. 根据用户消息中的关键词匹配
    2. 如果没有匹配到关键词，返回 None（不调用工具）

    Returns: (tool_def, args) 或 (None, None)
    """
    if not tools:
        return None, None

    best_match = None
    best_score = 0

    for tool_def in tools:
        func = tool_def.get("function", {})
        name = func.get("name", "")

        # 计算关键词匹配分数
        keywords = TOOL_KEYWORDS.get(name, [name])
        score = sum(1 for kw in keywords if kw.lower() in user_text.lower())

        if score > best_score:
            best_score = score
            best_match = tool_def

    if best_score == 0:
        # 没有关键词匹配，但请求携带了 tools，仍然选第一个工具
        # 这模拟 LLM 决定使用工具的场景
        best_match = tools[0]

    if best_match:
        tool_name = best_match["function"]["name"]
        args = build_tool_args(tool_name, user_text)
        return best_match, args

    return None, None


# ============ 响应构建 ============

def create_full_response(data):
    """构建非流式完整响应"""
    messages = data.get('messages', [])
    tools = data.get('tools')
    last_msg = messages[-1] if messages else {}

    tool_calls = None
    content = ""
    prompt_tokens = len(str(messages))

    # 1. 收到 tool 响应 → 不再调用工具，返回总结文本
    if last_msg.get("role") == "tool":
        tool_result = last_msg.get("content", "")
        content = f"根据查询结果，已为您处理完毕。工具返回信息: {tool_result[:200]}。如有其他问题请随时告诉我。"
        prompt_tokens = len(str(messages))

    # 2. 有 tools 参数 → 尝试选择工具调用
    elif tools and len(tools) > 0:
        # 获取所有用户消息的文本（用于提取 PII 参数）
        all_user_text = " ".join(
            msg.get("content", "") for msg in messages if msg.get("role") == "user"
        )
        last_user_text = last_msg.get("content", "") if last_msg.get("role") == "user" else all_user_text

        tool_def, args = select_tool(tools, all_user_text)

        if tool_def and args:
            func = tool_def["function"]
            tool_calls = [{
                "id": f"call_{uuid.uuid4().hex[:12]}",
                "type": "function",
                "index": 0,
                "function": {
                    "name": func["name"],
                    "arguments": json.dumps(args, ensure_ascii=False)
                }
            }]
            print(f"[MockLLM] 工具调用: {func['name']} 参数: {json.dumps(args, ensure_ascii=False)}")
        else:
            content = f"收到消息: {last_user_text}"

    # 3. 无 tools 参数 → 关键词匹配旧逻辑
    else:
        user_content = last_msg.get("content", "")
        if "天气" in user_content or "weather" in user_content.lower():
            tool_calls = [{
                "id": f"call_{uuid.uuid4().hex[:12]}",
                "type": "function",
                "index": 0,
                "function": {
                    "name": "get_weather",
                    "arguments": json.dumps({"city": extract_city(user_content) or "北京"}, ensure_ascii=False)
                }
            }]
        elif "搜索" in user_content or "search" in user_content.lower():
            tool_calls = [{
                "id": f"call_{uuid.uuid4().hex[:12]}",
                "type": "function",
                "index": 0,
                "function": {
                    "name": "search",
                    "arguments": json.dumps({"query": extract_query(user_content)}, ensure_ascii=False)
                }
            }]
        else:
            content = f"收到消息: {user_content}"

    # 构建响应
    completion_tokens = len(content) if content else len(json.dumps(tool_calls, ensure_ascii=False))

    response = {
        "id": generate_chat_id(),
        "object": "chat.completion",
        "created": int(datetime.now().timestamp()),
        "model": MODEL_NAME,
        "choices": [{
            "index": 0,
            "message": {
                "role": "assistant",
                "content": content if content else None
            },
            "finish_reason": "stop"
        }],
        "usage": make_usage(prompt_tokens, completion_tokens)
    }

    if tool_calls:
        response["choices"][0]["message"]["tool_calls"] = tool_calls
        response["choices"][0]["message"]["content"] = None
        response["choices"][0]["finish_reason"] = "tool_calls"
        # 修正 usage
        response["usage"]["completion_tokens"] = len(json.dumps(tool_calls, ensure_ascii=False))

    return response


def create_stream_response(data):
    """构建流式响应的生成器"""
    messages = data.get('messages', [])
    tools = data.get('tools')
    last_msg = messages[-1] if messages else {}

    # 流式模式下的工具调用：先发 tool_calls chunks，然后不继续（模拟）
    # 因为 Go 代理的流式工具调用处理比较复杂，流式模式主要测试文本遮罩

    # 如果有 tools 且不是 tool 响应，触发工具调用
    if tools and len(tools) > 0 and last_msg.get("role") != "tool":
        all_user_text = " ".join(
            msg.get("content", "") for msg in messages if msg.get("role") == "user"
        )
        tool_def, args = select_tool(tools, all_user_text)

        if tool_def and args:
            func = tool_def["function"]
            tool_call_id = f"call_{uuid.uuid4().hex[:12]}"
            arguments_str = json.dumps(args, ensure_ascii=False)

            print(f"[MockLLM Stream] 工具调用: {func['name']} 参数: {json.dumps(args, ensure_ascii=False)}")

            # 发送 tool_calls 的流式 chunks
            # chunk 1: delta 包含 tool_calls 开始（含 name）
            yield create_chunk({
                "index": 0,
                "delta": {
                    "tool_calls": [{
                        "index": 0,
                        "id": tool_call_id,
                        "type": "function",
                        "function": {
                            "name": func["name"],
                            "arguments": ""
                        }
                    }]
                },
                "finish_reason": None
            })
            time.sleep(0.02)

            # chunk 2: delta 包含 arguments 部分
            yield create_chunk({
                "index": 0,
                "delta": {
                    "tool_calls": [{
                        "index": 0,
                        "function": {
                            "arguments": arguments_str
                        }
                    }]
                },
                "finish_reason": None
            })
            time.sleep(0.02)

            # chunk 3: finish_reason = tool_calls
            yield create_chunk({
                "index": 0,
                "delta": {},
                "finish_reason": "tool_calls"
            })
            time.sleep(0.02)

            yield "data: [DONE]\n\n"
            return

    # 普通文本流式响应
    content = ""
    if last_msg.get("role") == "tool":
        tool_result = last_msg.get("content", "")
        content = f"根据查询结果，已为您处理完毕。工具返回信息: {tool_result[:200]}。"
    else:
        user_text = last_msg.get("content", "")
        if "天气" in user_text or "weather" in user_text.lower():
            content = f"{extract_city(user_text) or '北京'}今日天气晴，温度25度，湿度60%，东北风3级。"
        else:
            content = f"收到消息: {user_text}"

    # 分块发送
    for i in range(0, len(content), 5):
        chunk_text = content[i:i+5]
        yield create_chunk({
            "index": 0,
            "delta": {"content": chunk_text},
            "finish_reason": None
        })
        time.sleep(0.02)

    yield create_chunk({
        "index": 0,
        "delta": {},
        "finish_reason": "stop"
    })
    yield "data: [DONE]\n\n"


# ============ 路由 ============

@app.route("/health", methods=['GET'])
def health():
    return jsonify({"model": MODEL_NAME, "status": "ok"})


@app.route("/last_request", methods=['GET'])
def get_last_request():
    return jsonify(last_request_store.get("data") or {})


@app.route("/reset", methods=['POST'])
def reset():
    last_request_store["data"] = None
    return jsonify({"status": "ok"})


@app.route("/v1/chat/completions", methods=['POST'])
def chat_completions():
    global last_request_store
    try:
        data = request.get_json()
        last_request_store["data"] = data
        stream_mode = data.get('stream', False)

        if stream_mode:
            return Response(
                stream_with_context(create_stream_response(data)),
                mimetype='text/event-stream',
                headers={
                    'Cache-Control': 'no-cache',
                    'Connection': 'keep-alive'
                }
            )
        else:
            response = create_full_response(data)
            return jsonify(response)

    except Exception as e:
        print(f"[MockLLM Error] {e}")
        import traceback
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500


if __name__ == '__main__':
    print(f"Mock LLM starting on port 11434...")
    print(f"Supported tools: search, get_weather, calculate, get_user_info,")
    print(f"                 check_user_status, get_balance, transfer_money,")
    print(f"                 get_transactions, change_password")
    app.run(host='0.0.0.0', port=11437, debug=False, threaded=True)
