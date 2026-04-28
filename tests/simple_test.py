#!/usr/bin/env python3
"""简单的遮罩还原测试"""
import requests
import json
import time

BASE_URL = "http://127.0.0.1:8846"

def test_mask():
    """测试遮罩功能"""
    print("\n=== 遮罩功能测试 ===\n")
    
    test_cases = [
        ("我的手机号是13812345678", "中文手机号"),
        ("身份证号110101199003077777", "中文身份证"),
        ("邮箱test@example.com", "邮箱"),
        ("张三的电话是13900139000", "中文姓名+手机"),
    ]
    
    for text, desc in test_cases:
        payload = {
            "model": "test",
            "messages": [{"role": "user", "content": text}],
            "stream": False
        }
        
        start = time.time()
        try:
            resp = requests.post(
                f"{BASE_URL}/v1/chat/completions",
                json=payload,
                headers={"Content-Type": "application/json"},
                timeout=10
            )
            duration = (time.time() - start) * 1000
            
            if resp.status_code == 200:
                print(f"  [{desc}] 耗时: {duration:.2f}ms - OK")
            else:
                print(f"  [{desc}] 耗时: {duration:.2f}ms - 状态码: {resp.status_code}")
        except Exception as e:
            print(f"  [{desc}] 失败: {e}")

def test_health():
    """测试健康检查"""
    print("\n=== 健康检查 ===\n")
    
    try:
        resp = requests.get(f"{BASE_URL}/health", timeout=5)
        data = resp.json()
        print(f"  状态: {data.get('status')}")
        print(f"  活动引擎: {data.get('active_engine')}")
        print(f"  可用引擎: {list(data.get('pii_engines', {}).keys())}")
        return True
    except Exception as e:
        print(f"  错误: {e}")
        return False

if __name__ == "__main__":
    print("\nCloseMask 功能测试")
    print("=" * 40)
    
    if test_health():
        test_mask()
    
    print("\n测试完成\n")
