#!/usr/bin/env python3
"""
测试遮罩和还原效果
"""
import requests
import json
import time

BASE_URL = "http://127.0.0.1:8846"

def test_mask_restore():
    """测试遮罩和还原"""
    print("\n=== 遮罩还原测试 ===\n")
    
    # 测试用例
    test_text = "我的手机号是13812345678，身份证号110101199003077777，邮箱test@example.com"
    
    print(f"原文: {test_text}")
    
    payload = {
        "model": "deepseek-coder",
        "messages": [
            {"role": "user", "content": test_text}
        ],
        "stream": False,
        "max_tokens": 200
    }
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer sk-test-key"
    }
    
    start = time.time()
    resp = requests.post(
        f"{BASE_URL}/v1/chat/completions",
        json=payload,
        headers=headers,
        timeout=60
    )
    duration = (time.time() - start) * 1000
    
    if resp.status_code == 200:
        result = resp.json()
        content = result.get("choices", [{}])[0].get("message", {}).get("content", "")
        print(f"\nLLM 响应 (耗时 {duration:.0f}ms):")
        print(f"{content[:500]}...")
        
        # 检查是否包含占位符
        if "${" in content:
            print("\n[注意] 响应中包含未还原的占位符!")
        else:
            print("\n[OK] 占位符已正确还原")
    else:
        print(f"请求失败: {resp.status_code}")
        print(resp.text)

def test_with_without_ner():
    """测试有无 NER 的差异"""
    print("\n=== 有/无 NER 对比测试 ===\n")
    
    # 需要NER才能检测的内容
    test_text = "张三和李四今天去北京出差"
    
    print(f"原文: {test_text}")
    print("(需要 NER 才能检测人名和地名)")
    
    payload = {
        "model": "deepseek-coder",
        "messages": [
            {"role": "user", "content": test_text}
        ],
        "stream": False,
        "max_tokens": 100
    }
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer sk-test-key"
    }
    
    # 检查 NER 状态
    health = requests.get(f"{BASE_URL}/health").json()
    ner_available = health.get("pii_engines", {}).get("ner_service", {}).get("available", False)
    
    print(f"\nNER 服务状态: {'可用' if ner_available else '不可用'}")
    
    start = time.time()
    resp = requests.post(
        f"{BASE_URL}/v1/chat/completions",
        json=payload,
        headers=headers,
        timeout=60
    )
    duration = (time.time() - start) * 1000
    
    if resp.status_code == 200:
        result = resp.json()
        content = result.get("choices", [{}])[0].get("message", {}).get("content", "")
        print(f"\nLLM 响应 (耗时 {duration:.0f}ms):")
        print(f"{content[:300]}")
    else:
        print(f"请求失败: {resp.status_code}")

def test_local_mask_speed():
    """测试本地遮罩速度（不调用真实LLM）"""
    print("\n=== 本地遮罩处理速度测试 ===\n")
    
    # 使用 mock LLM 测试纯遮罩速度
    # 由于没有 mock LLM，这里只测试健康检查和基本请求
    
    test_cases = [
        ("短文本", "手机号13812345678"),
        ("中等文本", "用户张三的手机号是13812345678，身份证110101199003077777，邮箱test@example.com"),
        ("长文本", "用户张三的手机号是13812345678，身份证110101199003077777，邮箱test@example.com，银行卡6222021234567890123，地址北京市朝阳区xxx街道xxx号" * 3),
    ]
    
    for name, text in test_cases:
        payload = {
            "model": "test",
            "messages": [{"role": "user", "content": text}],
            "stream": False,
            "max_tokens": 50
        }
        
        times = []
        for i in range(3):
            start = time.time()
            resp = requests.post(
                f"{BASE_URL}/v1/chat/completions",
                json=payload,
                headers={"Content-Type": "application/json", "Authorization": "Bearer test"},
                timeout=30
            )
            times.append((time.time() - start) * 1000)
        
        avg = sum(times) / len(times)
        print(f"  {name} ({len(text)} 字符): 平均 {avg:.0f}ms")

if __name__ == "__main__":
    print("\n" + "="*60)
    print("  CloseMask 遮罩还原详细测试")
    print("="*60)
    
    test_mask_restore()
    test_with_without_ner()
    test_local_mask_speed()
    
    print("\n" + "="*60)
    print("  测试完成")
    print("="*60 + "\n")
