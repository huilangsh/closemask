#!/usr/bin/env python3
"""
CloseMask 完整功能测试
测试场景：
1. 有 NER 服务 vs 无 NER 服务
2. 遮罩和还原功能
3. 处理延迟统计
"""

import requests
import json
import time
import sys
from datetime import datetime

BASE_URL = "http://127.0.0.1:8846"

# 测试用例
TEST_CASES = [
    {
        "name": "中文手机号",
        "text": "我的手机号是13812345678，请帮我查询订单",
        "expected_types": ["PHONE"]
    },
    {
        "name": "中文身份证",
        "text": "身份证号110101199003077777需要验证",
        "expected_types": ["ID_CARD"]
    },
    {
        "name": "邮箱地址",
        "text": "联系邮箱zhang.san@example.com获取详情",
        "expected_types": ["EMAIL"]
    },
    {
        "name": "银行卡号",
        "text": "银行卡号6222021234567890123绑定成功",
        "expected_types": ["BANK_CARD"]
    },
    {
        "name": "API Key",
        "text": "API_KEY=sk-proj-abcdefghijklmnop123456",
        "expected_types": ["API_KEY", "CRED"]
    },
    {
        "name": "多PII组合",
        "text": "用户张三的手机号是13900139000，身份证110101199001011234，邮箱zhangsan@test.com",
        "expected_types": ["PHONE", "ID_CARD", "EMAIL"]
    },
    {
        "name": "人名检测(NER)",
        "text": "张三和李四今天去北京出差了",
        "expected_types": ["PER", "LOC"]  # 需要 NER
    },
    {
        "name": "组织名检测(NER)",
        "text": "腾讯公司和阿里巴巴集团达成合作",
        "expected_types": ["ORG"]
    },
]

def print_header(title):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}\n")

def check_health():
    """检查服务健康状态"""
    try:
        resp = requests.get(f"{BASE_URL}/health", timeout=5)
        data = resp.json()
        return data
    except Exception as e:
        return None

def test_single_case(case, session_id):
    """测试单个用例"""
    text = case["text"]
    name = case["name"]
    
    payload = {
        "model": "deepseek-coder",
        "messages": [
            {"role": "user", "content": text}
        ],
        "stream": False,
        "max_tokens": 100
    }
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer sk-test-key"  # 测试用密钥
    }
    
    # 记录开始时间
    start_time = time.time()
    
    try:
        resp = requests.post(
            f"{BASE_URL}/v1/chat/completions",
            json=payload,
            headers=headers,
            timeout=60
        )
        
        end_time = time.time()
        total_duration = (end_time - start_time) * 1000
        
        if resp.status_code != 200:
            return {
                "success": False,
                "error": f"HTTP {resp.status_code}",
                "duration": total_duration
            }
        
        result = resp.json()
        
        # 提取响应内容
        content = ""
        if "choices" in result and len(result["choices"]) > 0:
            content = result["choices"][0].get("message", {}).get("content", "")
        
        return {
            "success": True,
            "duration": total_duration,
            "response": content[:200] if content else "",
            "full_response": result
        }
        
    except Exception as e:
        return {
            "success": False,
            "error": str(e),
            "duration": 0
        }

def run_tests(with_ner=True):
    """运行测试"""
    print_header(f"功能测试 ({'有 NER' if with_ner else '无 NER'})")
    
    # 检查健康状态
    health = check_health()
    if not health:
        print("  [错误] 服务未启动")
        return
    
    print(f"  服务状态: {health.get('status')}")
    print(f"  活动引擎: {health.get('active_engine')}")
    
    engines = health.get('pii_engines', {})
    ner_available = engines.get('ner_service', {}).get('available', False)
    
    if with_ner and not ner_available:
        print("  [警告] NER 服务不可用，测试将跳过 NER 相关用例")
    
    print()
    
    results = []
    session_id = f"test_{int(time.time())}"
    
    for case in TEST_CASES:
        # 如果是无 NER 测试且用例需要 NER，跳过
        if not with_ner and "NER" in case["name"]:
            continue
        
        # 如果是有 NER 测试但 NER 不可用，标记
        if with_ner and not ner_available and "NER" in case["name"]:
            print(f"  [{case['name']}] 跳过 (NER 不可用)")
            continue
        
        result = test_single_case(case, session_id)
        
        status = "[OK]" if result["success"] else "[FAIL]"
        duration = result.get("duration", 0)
        
        print(f"  [{case['name']}] {status} 耗时: {duration:.0f}ms")
        
        if result["success"] and result.get("response"):
            print(f"      响应: {result['response'][:100]}...")
        
        if not result["success"]:
            print(f"      错误: {result.get('error', 'unknown')}")
        
        results.append({
            "name": case["name"],
            **result
        })
    
    # 统计
    print_header("性能统计")
    
    successful = [r for r in results if r["success"]]
    failed = [r for r in results if not r["success"]]
    
    if successful:
        durations = [r["duration"] for r in successful]
        avg_duration = sum(durations) / len(durations)
        min_duration = min(durations)
        max_duration = max(durations)
        
        print(f"  成功: {len(successful)}/{len(results)}")
        print(f"  平均延迟: {avg_duration:.0f}ms")
        print(f"  最小延迟: {min_duration:.0f}ms")
        print(f"  最大延迟: {max_duration:.0f}ms")
    
    if failed:
        print(f"\n  失败用例:")
        for f in failed:
            print(f"    - {f['name']}: {f.get('error', 'unknown')}")

def main():
    print(f"\n{'='*60}")
    print(f"  CloseMask 完整功能测试")
    print(f"  时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"{'='*60}")
    
    # 检查服务
    health = check_health()
    if not health:
        print("\n  [错误] CloseMask 服务未启动")
        print("  请先运行: closemask.exe -config config.json")
        sys.exit(1)
    
    # 运行测试
    run_tests(with_ner=True)
    
    print(f"\n{'='*60}")
    print(f"  测试完成")
    print(f"{'='*60}\n")

if __name__ == "__main__":
    main()
