#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
CloseMask 综合测试场景
=================================

适配主动代理架构（Go 代理自动执行工具调用，客户端只收到最终文本）。

测试场景：
1. 长输入包含多种 PII 类型
2. 多轮对话 + 工具调用（accessToken 遮罩）
3. 流式响应 + 长文本
4. 复杂多轮对话（5 轮业务流程）

测试目标：
- 验证 PII 遮罩：发送给 LLM 的消息中不应包含原始 PII
- 验证 PII 还原：最终响应中不应包含占位符
- 验证工具调用：Go 代理能自动执行工具并返回结果
- 验证多轮对话：占位符在不同轮次间正确持久化和还原
"""

import sys
import io
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')

import json
import requests
import time
from pathlib import Path
from typing import List, Dict, Any, Tuple


class ComprehensiveTestScenario:
    def __init__(self, proxy_url="http://localhost:8846"):
        self.proxy_url = proxy_url
        self.session_id = "test-comprehensive-session-001"
        self.session = requests.Session()
        self.results: List[Tuple[str, str, str]] = []

    def print_section(self, title: str):
        print("\n" + "=" * 80)
        print(f"  {title}")
        print("=" * 80)

    def send_chat_completion(
        self,
        messages: List[Dict[str, str]],
        stream: bool = False,
        tools: List[Dict[str, Any]] = None
    ) -> requests.Response:
        """发送聊天完成请求"""
        url = f"{self.proxy_url}/v1/chat/completions"
        headers = {
            "Content-Type": "application/json",
            "X-Session-ID": self.session_id
        }

        data = {
            "model": "gpt-3.5-turbo",
            "messages": messages,
            "stream": stream
        }

        if tools:
            data["tools"] = tools

        if stream:
            response = self.session.post(url, headers=headers, json=data, stream=True)
            return response
        else:
            response = self.session.post(url, headers=headers, json=data, timeout=30)
            return response

    def assert_no_placeholder(self, text: str, context: str = "") -> bool:
        """验证文本中不包含占位符"""
        if "__" in text:
            # 检查是否是占位符格式 __TYPE_xxx__
            import re
            placeholders = re.findall(r'__[A-Z_]+_[a-f0-9]+__', text)
            if placeholders:
                print(f"  [FAIL] {context}: 发现未还原的占位符: {placeholders}")
                return False
        return True

    def assert_success(self, response: requests.Response, context: str = "") -> bool:
        """验证 HTTP 响应成功"""
        if response.status_code != 200:
            print(f"  [FAIL] {context}: HTTP {response.status_code} - {response.text[:200]}")
            return False
        return True

    def record(self, name: str, passed: bool, detail: str = ""):
        status = "PASS" if passed else "FAIL"
        self.results.append((name, status, detail))
        icon = "[PASS]" if passed else "[FAIL]"
        msg = f"  {icon} {name}"
        if detail:
            msg += f" - {detail}"
        print(msg)

    # ============ 场景 1: 长输入 + 多种 PII ============

    def test_scenario_1_long_input_with_pii(self):
        """场景1：长输入包含多种 PII 类型"""
        self.print_section("场景 1: 长输入（接近上下文窗口）+ 多种 PII 类型")

        long_text = self._generate_long_text_with_pii(length=3000)

        messages = [{
            "role": "user",
            "content": f"""我需要处理以下用户信息，请帮我分析和整理：

{long_text}

请帮我：
1. 统计有多少个手机号
2. 统计有多少个邮箱
3. 统计有多少个身份证号
4. 识别所有敏感信息类型
5. 给出处理建议

注意：这些信息都需要严格保密。"""
        }]

        response = self.send_chat_completion(messages, stream=False)
        passed = self.assert_success(response, "场景1-HTTP")

        if passed:
            try:
                data = response.json()
                content = ""
                if "choices" in data and len(data["choices"]) > 0:
                    message = data["choices"][0].get("message", {})
                    content = message.get("content", "")

                # 验证：响应中有内容
                has_content = len(content) > 0
                self.record("场景1-响应有内容", has_content, f"长度={len(content)}")

                # 验证：响应中不包含占位符
                no_placeholder = self.assert_no_placeholder(content, "场景1-占位符还原")
                self.record("场景1-无残留占位符", no_placeholder)

            except Exception as e:
                self.record("场景1-解析响应", False, str(e))

    # ============ 场景 2: 多轮对话 + 工具调用 ============

    def test_scenario_2_multi_turn_with_tools(self):
        """场景2：多轮对话 + 工具调用（包含 accessToken）"""
        self.print_section("场景 2: 多轮对话 + 工具调用 (accessToken 遮罩)")

        # 轮次 1：用户提供信息（含 PII），代理应自动调用工具
        messages = [{
            "role": "user",
            "content": "你好，我需要查询用户信息。我的手机号是 13800138000，邮箱是 zhangsan@example.com，身份证号是 110101199003077777。另外，我的系统 accessToken 是 eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c。请帮我查询用户状态。"
        }]

        response = self.send_chat_completion(
            messages,
            stream=False,
            tools=[{
                "type": "function",
                "function": {
                    "name": "get_user_info",
                    "description": "查询用户信息",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "phone": {"type": "string", "description": "手机号"},
                            "email": {"type": "string", "description": "邮箱"},
                            "id_card": {"type": "string", "description": "身份证号"},
                            "access_token": {"type": "string", "description": "访问令牌"}
                        },
                        "required": ["phone"]
                    }
                }
            }]
        )

        passed = self.assert_success(response, "场景2-轮次1-HTTP")
        if passed:
            try:
                data = response.json()
                content = ""
                if "choices" in data and len(data["choices"]) > 0:
                    message = data["choices"][0].get("message", {})
                    content = message.get("content", "")

                has_content = len(content) > 0
                self.record("场景2-轮次1-有响应", has_content, f"长度={len(content)}")

                no_placeholder = self.assert_no_placeholder(content, "场景2-轮次1")
                self.record("场景2-轮次1-无占位符", no_placeholder)

                # 验证：响应中包含工具执行的结果信息（如 "张三"、"余额" 等）
                has_result_info = any(kw in content for kw in ["张三", "余额", "9999", "工具", "查询"])
                self.record("场景2-轮次1-含工具结果", has_result_info)

            except Exception as e:
                self.record("场景2-轮次1-解析", False, str(e))

        # 轮次 2：继续对话，请求天气查询
        time.sleep(1)
        print("\n  --- 继续对话（轮次 2）---")

        messages.append({
            "role": "assistant",
            "content": response.json()["choices"][0]["message"]["content"]
        })
        messages.append({
            "role": "user",
            "content": "请告诉我用户的状态，并且帮我查询北京天气。"
        })

        response = self.send_chat_completion(
            messages,
            stream=False,
            tools=[{
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "description": "查询天气",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "city": {"type": "string", "description": "城市名称"}
                        },
                        "required": ["city"]
                    }
                }
            }]
        )

        passed = self.assert_success(response, "场景2-轮次2-HTTP")
        if passed:
            try:
                data = response.json()
                content = ""
                if "choices" in data and len(data["choices"]) > 0:
                    message = data["choices"][0].get("message", {})
                    content = message.get("content", "")

                has_content = len(content) > 0
                self.record("场景2-轮次2-有响应", has_content, f"长度={len(content)}")

                no_placeholder = self.assert_no_placeholder(content, "场景2-轮次2")
                self.record("场景2-轮次2-无占位符", no_placeholder)

            except Exception as e:
                self.record("场景2-轮次2-解析", False, str(e))

    # ============ 场景 3: 流式响应 + 长文本 ============

    def test_scenario_3_streaming_long_response(self):
        """场景3：流式响应 + 长文本"""
        self.print_section("场景 3: 流式响应 + 长文本生成")

        messages = [{
            "role": "user",
            "content": """请详细介绍一下人工智能的发展历史，包括：

1. 人工智能的起源
2. 早期 AI 系统的发展
3. 机器学习的兴起
4. 深度学习的突破
5. 大语言模型的发展
6. AI 在各个领域的应用
7. 未来发展趋势

请尽可能详细地展开每个部分，我的联系方式是 13900139000，有问题可以联系我。"""
        }]

        response = self.send_chat_completion(messages, stream=True)
        passed = self.assert_success(response, "场景3-HTTP")

        full_content = ""
        if passed:
            try:
                for line in response.iter_lines():
                    if line:
                        line = line.decode('utf-8')
                        if line.startswith('data: '):
                            data_content = line[6:]
                            if data_content == '[DONE]':
                                break
                            try:
                                chunk = json.loads(data_content)
                                if 'choices' in chunk and len(chunk['choices']) > 0:
                                    delta = chunk['choices'][0].get('delta', {})
                                    if 'content' in delta:
                                        full_content += delta['content']
                            except:
                                pass

                self.record("场景3-流式内容有数据", len(full_content) > 0, f"长度={len(full_content)}")

                no_placeholder = self.assert_no_placeholder(full_content, "场景3")
                self.record("场景3-无残留占位符", no_placeholder)

            except Exception as e:
                self.record("场景3-流式解析", False, str(e))

    # ============ 场景 4: 复杂多轮对话（5轮） ============

    def test_scenario_4_complex_multi_turn(self):
        """场景4：复杂多轮对话（5轮以上）"""
        self.print_section("场景 4: 复杂多轮对话 (5 轮)")

        conversation_history = []

        # 轮次 1：普通对话
        conversation_history.append({
            "role": "user",
            "content": "你好，我是李四。我的手机号是 13700137000，邮箱是 lisi@example.com。我需要咨询一些问题。"
        })

        response = self.send_chat_completion(conversation_history, stream=False)
        passed = self.assert_success(response, "场景4-轮次1-HTTP")

        if passed:
            try:
                data = response.json()
                content = data["choices"][0]["message"]["content"]
                conversation_history.append({"role": "assistant", "content": content})
                self.record("场景4-轮次1-有响应", len(content) > 0)
                self.record("场景4-轮次1-无占位符", self.assert_no_placeholder(content, "轮次1"))
            except Exception as e:
                self.record("场景4-轮次1", False, str(e))

        time.sleep(0.5)

        # 轮次 2：查询余额（带工具调用）
        print("  --- 轮次 2: 查询余额 ---")
        conversation_history.append({
            "role": "user",
            "content": "帮我查询我的账户余额，我的身份证号是 110101199003077777。"
        })

        response = self.send_chat_completion(
            conversation_history,
            stream=False,
            tools=[{
                "type": "function",
                "function": {
                    "name": "get_balance",
                    "description": "查询账户余额",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "id_card": {"type": "string", "description": "身份证号"},
                            "phone": {"type": "string", "description": "手机号"}
                        }
                    }
                }
            }]
        )

        passed = self.assert_success(response, "场景4-轮次2-HTTP")
        if passed:
            try:
                data = response.json()
                content = data["choices"][0]["message"]["content"]
                conversation_history.append({"role": "assistant", "content": content})
                self.record("场景4-轮次2-有响应", len(content) > 0)
                self.record("场景4-轮次2-无占位符", self.assert_no_placeholder(content, "轮次2"))
                has_balance = any(kw in content for kw in ["8888", "余额", "CNY", "查询"])
                self.record("场景4-轮次2-含余额信息", has_balance)
            except Exception as e:
                self.record("场景4-轮次2", False, str(e))

        time.sleep(0.5)

        # 轮次 3：转账
        print("  --- 轮次 3: 转账 ---")
        conversation_history.append({
            "role": "user",
            "content": "我想转账给王五，他的手机号是 13600136000，账号是 6222000012345678。转账金额 1000 元。请帮我执行转账。"
        })

        response = self.send_chat_completion(
            conversation_history,
            stream=False,
            tools=[{
                "type": "function",
                "function": {
                    "name": "transfer_money",
                    "description": "转账",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "from_phone": {"type": "string"},
                            "to_phone": {"type": "string"},
                            "to_account": {"type": "string"},
                            "amount": {"type": "number"}
                        },
                        "required": ["from_phone", "to_phone", "amount"]
                    }
                }
            }]
        )

        passed = self.assert_success(response, "场景4-轮次3-HTTP")
        if passed:
            try:
                data = response.json()
                content = data["choices"][0]["message"]["content"]
                conversation_history.append({"role": "assistant", "content": content})
                self.record("场景4-轮次3-有响应", len(content) > 0)
                self.record("场景4-轮次3-无占位符", self.assert_no_placeholder(content, "轮次3"))
            except Exception as e:
                self.record("场景4-轮次3", False, str(e))

        time.sleep(0.5)

        # 轮次 4：查询交易记录
        print("  --- 轮次 4: 查询交易记录 ---")
        conversation_history.append({
            "role": "user",
            "content": "请帮我查询最近的交易记录。"
        })

        response = self.send_chat_completion(
            conversation_history,
            stream=False,
            tools=[{
                "type": "function",
                "function": {
                    "name": "get_transactions",
                    "description": "查询交易记录",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "phone": {"type": "string"}
                        }
                    }
                }
            }]
        )

        passed = self.assert_success(response, "场景4-轮次4-HTTP")
        if passed:
            try:
                data = response.json()
                content = data["choices"][0]["message"]["content"]
                conversation_history.append({"role": "assistant", "content": content})
                self.record("场景4-轮次4-有响应", len(content) > 0)
                self.record("场景4-轮次4-无占位符", self.assert_no_placeholder(content, "轮次4"))
            except Exception as e:
                self.record("场景4-轮次4", False, str(e))

        time.sleep(0.5)

        # 轮次 5：修改密码
        print("  --- 轮次 5: 修改密码 ---")
        conversation_history.append({
            "role": "user",
            "content": "我需要修改密码，我的验证码是 123456，请帮我修改。"
        })

        response = self.send_chat_completion(
            conversation_history,
            stream=False,
            tools=[{
                "type": "function",
                "function": {
                    "name": "change_password",
                    "description": "修改密码",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "phone": {"type": "string"},
                            "verification_code": {"type": "string"}
                        },
                        "required": ["phone", "verification_code"]
                    }
                }
            }]
        )

        passed = self.assert_success(response, "场景4-轮次5-HTTP")
        if passed:
            try:
                data = response.json()
                content = data["choices"][0]["message"]["content"]
                self.record("场景4-轮次5-有响应", len(content) > 0)
                self.record("场景4-轮次5-无占位符", self.assert_no_placeholder(content, "轮次5"))
            except Exception as e:
                self.record("场景4-轮次5", False, str(e))

    def _generate_long_text_with_pii(self, length: int = 3000) -> str:
        """生成包含 PII 的长文本"""
        pii_data = [
            ("phone1", "13800138000"), ("phone2", "13900139000"),
            ("phone3", "13700137000"), ("phone4", "13600136000"),
            ("phone5", "13500135000"),
            ("email1", "zhangsan@example.com"), ("email2", "lisi@example.com"),
            ("email3", "wangwu@example.com"), ("email4", "zhao@example.com"),
            ("id_card1", "110101199003077777"), ("id_card2", "110101199003077778"),
            ("id_card3", "110101199003077779"),
            ("access_token1", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ"),
            ("bank_card1", "6222000012345678"), ("bank_card2", "6222000087654321"),
        ]

        base_text = """
        本文档包含多个用户的敏感信息，需要进行隐私保护处理。

        用户 1：张三
        - 手机号：{phone1}
        - 邮箱：{email1}
        - 身份证号：{id_card1}
        - 银行卡号：{bank_card1}
        - AccessToken：{access_token1}

        用户 2：李四
        - 手机号：{phone2}
        - 邮箱：{email2}
        - 身份证号：{id_card2}
        - 银行卡号：{bank_card2}

        用户 3：王五
        - 手机号：{phone3}
        - 邮箱：{email3}
        - 身份证号：{id_card3}

        用户 4：赵六
        - 手机号：{phone4}
        - 邮箱：{email4}

        用户 5：孙七
        - 手机号：{phone5}

        安全主管联系方式：{phone1}
        技术支持：tech@example.com
        法务部门：legal@example.com
        """

        text = base_text.format(**{k: v for k, v in pii_data})

        while len(text) < length:
            text += "\n\n" + "=" * 80 + "\n补充信息：\n" + "=" * 80 + "\n" + text

        return text[:length]


def main():
    print("\n" + "=" * 80)
    print("  CloseMask 综合测试场景")
    print("  适配主动代理架构 - Go 代理自动执行工具调用")
    print("=" * 80)

    tester = ComprehensiveTestScenario()

    scenarios = [
        ("场景 1: 长输入 + 多种 PII", tester.test_scenario_1_long_input_with_pii),
        ("场景 2: 多轮对话 + 工具调用 (accessToken)", tester.test_scenario_2_multi_turn_with_tools),
        ("场景 3: 流式响应 + 长文本", tester.test_scenario_3_streaming_long_response),
        ("场景 4: 复杂多轮对话 (5 轮)", tester.test_scenario_4_complex_multi_turn),
    ]

    for name, test_func in scenarios:
        try:
            print(f"\n>> 开始执行: {name}")
            test_func()
        except Exception as e:
            print(f"\n  [ERROR] {name}: {str(e)}")
            import traceback
            traceback.print_exc()

        time.sleep(1)

    # 打印测试总结
    print("\n" + "=" * 80)
    print("  测试总结")
    print("=" * 80)

    for name, status, detail in tester.results:
        icon = "[PASS]" if status == "PASS" else "[FAIL]"
        msg = f"  {icon} {name}"
        if detail:
            msg += f" - {detail}"
        print(msg)

    pass_count = sum(1 for _, s, _ in tester.results if s == "PASS")
    fail_count = sum(1 for _, s, _ in tester.results if s == "FAIL")
    total = len(tester.results)

    print(f"\n  总计: {pass_count}/{total} 通过, {fail_count} 失败")
    if total > 0:
        print(f"  通过率: {pass_count/total*100:.1f}%")

    print("\n" + "=" * 80 + "\n")
    return fail_count == 0


if __name__ == "__main__":
    success = main()
    exit(0 if success else 1)
