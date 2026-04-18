#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
CloseMask 综合测试脚本
覆盖所有核心功能：PII 遮罩、占位符还原、流式响应、工具调用
"""

import requests
import json
import re
import time
import sys
import io
import subprocess
import os
import socket

sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')

PROXY_URL = "http://localhost:8846/v1/chat/completions"
AIFW_MASK_URL = "http://localhost:8845/api/mask_text"
AIFW_RESTORE_URL = "http://localhost:8845/api/restore_text"

test_results = []
round_id = 0


def log(category, name, passed, detail=""):
    test_results.append({
        "round": round_id,
        "category": category,
        "name": name,
        "passed": passed,
        "detail": detail
    })
    icon = "[OK]" if passed else "[FAIL]"
    msg = f"  {icon} {name}"
    if detail:
        msg += f" - {detail}"
    print(msg)


def is_port_open(port, timeout=1):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(timeout)
    r = s.connect_ex(("127.0.0.1", port)) == 0
    s.close()
    return r


def check_services():
    """检查所有服务是否运行"""
    services = {
        "AIFW (8845)": 8845,
        "Mock LLM (11437)": 11437,
        "Proxy (8846)": 8846,
    }
    all_ok = True
    for name, port in services.items():
        if is_port_open(port):
            print(f"  [OK] {name}")
        else:
            print(f"  [FAIL] {name} - NOT RUNNING")
            all_ok = False
    return all_ok


# ==================== 测试组 1: AIFW 遮罩服务 ====================

def test_aifw_masking():
    """测试 AIFW 遮罩服务的基本功能"""
    print("\n  --- 1.1 AIFW 遮罩/还原基本功能 ---")

    cases = [
        ("手机号", "我的电话是 13812345678", "13812345678"),
        ("身份证", "身份证号 110101199001011234", "110101199001011234"),
        ("邮箱", "邮箱 test@example.com", "test@example.com"),
        ("多PII", "phone:13812345678 email:abc@def.com", "13812345678"),
    ]

    for label, text, expected_pii in cases:
        resp = requests.post(AIFW_MASK_URL, json={"text": text}, timeout=10)
        if resp.status_code != 200:
            log("aifw", label, False, f"HTTP {resp.status_code}")
            continue

        data = resp.json()
        masked = data["output"]["text"]
        mask_meta = data["output"].get("maskMeta", "")

        # PII 应该被移除
        pii_removed = expected_pii not in masked
        log("aifw", f"{label}_pii_removed", pii_removed)

        # 应该包含占位符
        has_placeholder = "__" in masked
        log("aifw", f"{label}_has_placeholder", has_placeholder, masked[:80])

        # 还原应该返回原始文本
        restore_resp = requests.post(AIFW_RESTORE_URL, json={
            "text": masked, "maskMeta": mask_meta
        }, timeout=10)
        if restore_resp.status_code == 200:
            restored = restore_resp.json()["output"]["text"]
            pii_back = expected_pii in restored
            log("aifw", f"{label}_restored", pii_back)


# ==================== 测试组 2: 代理 - 非流式请求 ====================

def test_proxy_non_stream():
    """测试代理的非流式 PII 遮罩和还原"""
    print("\n  --- 2.1 代理非流式 PII 遮罩/还原 ---")

    cases = [
        ("手机号", "13812345678", "13812345678"),
        ("复合文本", "电话13812345678邮箱abc@test.com", ["13812345678", "abc@test.com"]),
        ("中文句子", "帮我查一下13812345678这个号码", "13812345678"),
    ]

    for label, text, expected in cases:
        resp = requests.post(PROXY_URL, json={
            "model": "gpt-3.5-turbo",
            "messages": [{"role": "user", "content": text}],
            "stream": False
        }, timeout=30)

        if resp.status_code != 200:
            log("proxy", f"nonstream_{label}", False, f"HTTP {resp.status_code}")
            continue

        data = resp.json()
        content = data["choices"][0]["message"]["content"]

        if isinstance(expected, list):
            expected_list = expected
        else:
            expected_list = [expected]

        # PII 应该在最终响应中被还原
        all_restored = all(pii in content for pii in expected_list)
        log("proxy", f"nonstream_{label}_restored", all_restored, content[:80])

        # 不应有占位符泄露
        has_placeholder = bool(re.search(r'__[A-Z]+_[a-f0-9]+__', content))
        log("proxy", f"nonstream_{label}_no_leak", not has_placeholder,
            "占位符泄露!" if has_placeholder else "无泄露")


# ==================== 测试组 3: 代理 - 流式请求 ====================

def test_proxy_stream():
    """测试代理的流式 PII 遮罩和还原（核心场景）"""
    print("\n  --- 3.1 流式响应占位符还原 ---")

    cases = [
        ("手机号", "13812345678", "13812345678"),
        ("邮箱", "abc@test.com", "abc@test.com"),
        ("中文句子", "我的号码是13812345678", "13812345678"),
    ]

    for label, text, expected in cases:
        resp = requests.post(PROXY_URL, json={
            "model": "gpt-3.5-turbo",
            "messages": [{"role": "user", "content": text}],
            "stream": True
        }, timeout=60, stream=True)

        if resp.status_code != 200:
            log("proxy_stream", label, False, f"HTTP {resp.status_code}")
            continue

        full_content = ""
        chunk_count = 0
        for line in resp.iter_lines(decode_unicode=True):
            if line:
                chunk_count += 1
                if line.startswith("data: ") and line != "data: [DONE]":
                    try:
                        chunk = json.loads(line[6:])
                        delta = chunk.get("choices", [{}])[0].get("delta", {})
                        if "content" in delta and delta["content"]:
                            full_content += delta["content"]
                    except Exception:
                        pass

        # PII 应该被还原
        pii_restored = expected in full_content
        log("proxy_stream", f"{label}_restored", pii_restored, full_content[:80])

        # 不应有占位符泄露
        has_placeholder = bool(re.search(r'__[A-Z]+_[a-f0-9]+__', full_content))
        log("proxy_stream", f"{label}_no_leak", not has_placeholder,
            "占位符泄露!" if has_placeholder else "无泄露")


# ==================== 测试组 4: 非 PII 文本不被误遮罩 ====================

def test_non_pii():
    """不含 PII 的文本不应被修改"""
    print("\n  --- 4.1 非 PII 文本保护 ---")

    texts = [
        "hello world",
        "这是一个普通的消息",
        "the temperature is 25 degrees",
    ]

    for i, text in enumerate(texts):
        # 先检查 AIFW 是否修改了文本
        resp = requests.post(AIFW_MASK_URL, json={"text": text}, timeout=10)
        if resp.status_code == 200:
            masked = resp.json()["output"]["text"]
            unchanged = (masked == text)
            log("non_pii", f"aifw_text_{i+1}_unchanged", unchanged)
        else:
            log("non_pii", f"aifw_text_{i+1}_unchanged", False)

        # 再检查代理是否修改了文本
        resp = requests.post(PROXY_URL, json={
            "model": "gpt-3.5-turbo",
            "messages": [{"role": "user", "content": text}],
            "stream": False
        }, timeout=30)

        if resp.status_code == 200:
            content = resp.json()["choices"][0]["message"]["content"]
            text_intact = text in content
            log("non_pii", f"proxy_text_{i+1}_intact", text_intact)


# ==================== 测试组 5: 多种 PII 混合 ====================

def test_mixed_pii():
    """一次请求中包含多种 PII 类型"""
    print("\n  --- 5.1 混合 PII 类型 ---")

    text = "phone:13812345678 id:110101199001011234 email:abc@test.com"
    piis = ["13812345678", "110101199001011234", "abc@test.com"]

    # 非流式
    resp = requests.post(PROXY_URL, json={
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": text}],
        "stream": False
    }, timeout=30)

    if resp.status_code == 200:
        content = resp.json()["choices"][0]["message"]["content"]
        all_restored = all(pii in content for pii in piis)
        log("mixed_pii", "nonstream_all_restored", all_restored)

        has_placeholder = bool(re.search(r'__[A-Z]+_[a-f0-9]+__', content))
        log("mixed_pii", "nonstream_no_leak", not has_placeholder)

    # 流式
    resp = requests.post(PROXY_URL, json={
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": text}],
        "stream": True
    }, timeout=60, stream=True)

    if resp.status_code == 200:
        full_content = ""
        for line in resp.iter_lines(decode_unicode=True):
            if line and line.startswith("data: ") and line != "data: [DONE]":
                try:
                    chunk = json.loads(line[6:])
                    delta = chunk.get("choices", [{}])[0].get("delta", {})
                    if "content" in delta and delta["content"]:
                        full_content += delta["content"]
                except Exception:
                    pass

        all_restored = all(pii in full_content for pii in piis)
        log("mixed_pii", "stream_all_restored", all_restored)

        has_placeholder = bool(re.search(r'__[A-Z]+_[a-f0-9]+__', full_content))
        log("mixed_pii", "stream_no_leak", not has_placeholder)


# ==================== 测试组 6: 代理 - 工具调用（流式） ====================

def test_proxy_tool_call_stream():
    """测试流式模式下的工具调用流程"""
    print("\n  --- 6.1 流式工具调用 ---")

    phone = "13812345678"
    text = f"查询用户信息 {phone}"

    resp = requests.post(PROXY_URL, json={
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": text}],
        "stream": True,
        "tools": [{
            "type": "function",
            "function": {
                "name": "get_user_info",
                "description": "Get user info",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "phone": {"type": "string"}
                    }
                }
            }
        }]
    }, timeout=60, stream=True)

    if resp.status_code != 200:
        log("tool_stream", "request", False, f"HTTP {resp.status_code}")
        return

    log("tool_stream", "request", True, f"HTTP {resp.status_code}")

    full_content = ""
    has_tool_calls = False

    for line in resp.iter_lines(decode_unicode=True):
        if line and line.startswith("data: ") and line != "data: [DONE]":
            try:
                chunk = json.loads(line[6:])
                delta = chunk.get("choices", [{}])[0].get("delta", {})
                # 检查是否有 tool_calls
                if "tool_calls" in delta:
                    has_tool_calls = True
                if "content" in delta and delta["content"]:
                    full_content += delta["content"]
            except Exception:
                pass

    log("tool_stream", "has_tool_calls", has_tool_calls)

    # 如果有最终文本（工具调用后的回复），检查 PII 还原
    if full_content and phone in full_content:
        log("tool_stream", "pii_in_final_response", True)
    elif full_content:
        log("tool_stream", "pii_in_final_response", False,
            f"内容: {full_content[:80]}")
    else:
        log("tool_stream", "pii_in_final_response", False, "无文本内容返回（纯 tool_calls）")


# ==================== 测试组 7: 代理 - 工具调用（非流式） ====================

def test_proxy_tool_call_non_stream():
    """测试非流式模式下的工具调用流程"""
    print("\n  --- 7.1 非流式工具调用 ---")

    phone = "13812345678"
    text = f"查询用户信息 {phone}"

    resp = requests.post(PROXY_URL, json={
        "model": "gpt-3.5-turbo",
        "messages": [{"role": "user", "content": text}],
        "stream": False,
        "tools": [{
            "type": "function",
            "function": {
                "name": "get_user_info",
                "description": "Get user info",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "phone": {"type": "string"}
                    }
                }
            }
        }]
    }, timeout=30)

    if resp.status_code != 200:
        log("tool_nonstream", "request", False, f"HTTP {resp.status_code}")
        return

    data = resp.json()
    choice = data["choices"][0]
    finish = choice.get("finish_reason", "")

    log("tool_nonstream", "request", True, f"finish_reason={finish}")

    message = choice.get("message", {})
    content = message.get("content", "")

    if content:
        has_placeholder = bool(re.search(r'__[A-Z]+_[a-f0-9]+__', content))
        log("tool_nonstream", "no_placeholder_leak", not has_placeholder)
    else:
        log("tool_nonstream", "no_placeholder_leak", True, "无内容字段")


# ==================== 主函数 ====================

def run_test_round(round_num):
    global round_id, test_results
    round_id = round_num

    print(f"\n{'='*70}")
    print(f"  第 {round_num} 轮测试  -  {time.strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"{'='*70}")

    if not check_services():
        print("\n  服务未全部启动，跳过本轮测试。")
        return False

    test_aifw_masking()
    test_proxy_non_stream()
    test_proxy_stream()
    test_non_pii()
    test_mixed_pii()
    test_proxy_tool_call_stream()
    test_proxy_tool_call_non_stream()

    return True


def print_summary():
    passed = sum(1 for r in test_results if r["passed"])
    failed = sum(1 for r in test_results if not r["passed"])
    total = len(test_results)

    print(f"\n{'='*70}")
    print(f"  测试总结  -  {time.strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"{'='*70}")
    print(f"\n  总计: {total}  通过: {passed}  失败: {failed}  通过率: {passed/total*100:.1f}%")

    if failed > 0:
        print(f"\n  失败项目:")
        for r in test_results:
            if not r["passed"]:
                print(f"    [FAIL] Round {r['round']} | {r['category']}/{r['name']} | {r['detail']}")
    else:
        print(f"\n  全部通过。")

    return failed == 0


def save_results(filepath):
    with open(filepath, "w", encoding="utf-8") as f:
        json.dump(test_results, f, ensure_ascii=False, indent=2)
    print(f"\n  结果已保存: {filepath}")


if __name__ == "__main__":
    ok1 = run_test_round(1)
    if ok1:
        time.sleep(2)
        ok2 = run_test_round(2)

    all_passed = print_summary()
    save_results("d:/mask/test_results_final.json")

    sys.exit(0 if all_passed else 1)
