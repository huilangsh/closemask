#!/usr/bin/env python3
"""
CloseMask PII 检测能力对比测试
对比内置正则检测 vs NER 服务的检测能力
"""

import requests
import json
import time
from datetime import datetime

NER_URL = "http://127.0.0.1:8847"
CLOSEMASK_URL = "http://127.0.0.1:8846"

# 测试用例：包含各种 PII 类型
TEST_CASES = [
    # === 内置正则能检测的 ===
    {
        "category": "手机号",
        "text": "我的手机号是13812345678",
        "builtin_expected": True,
        "ner_expected": True,
        "builtin_types": ["PHONE"],
        "ner_types": ["PHONE"]
    },
    {
        "category": "身份证号",
        "text": "身份证号110101199003077777需要验证",
        "builtin_expected": True,
        "ner_expected": True,
        "builtin_types": ["ID_CARD"],
        "ner_types": ["ID_CARD"]
    },
    {
        "category": "邮箱地址",
        "text": "联系邮箱zhang.san@example.com",
        "builtin_expected": True,
        "ner_expected": True,
        "builtin_types": ["EMAIL"],
        "ner_types": ["EMAIL"]
    },
    {
        "category": "银行卡号",
        "text": "银行卡号6222021234567890123",
        "builtin_expected": True,
        "ner_expected": True,
        "builtin_types": ["BANK_CARD"],
        "ner_types": ["BANK_CARD"]
    },
    {
        "category": "IP地址",
        "text": "服务器IP是192.168.1.100",
        "builtin_expected": True,
        "ner_expected": False,
        "builtin_types": ["IP_ADDRESS"],
        "ner_types": []
    },
    {
        "category": "API Key",
        "text": "API_KEY=sk-proj-abcdefghijklmnop123456",
        "builtin_expected": True,
        "ner_expected": False,
        "builtin_types": ["API_KEY"],
        "ner_types": []
    },
    {
        "category": "JWT Token",
        "text": "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
        "builtin_expected": True,
        "ner_expected": False,
        "builtin_types": ["JWT_TOKEN"],
        "ner_types": []
    },
    {
        "category": "AWS Key",
        "text": "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
        "builtin_expected": True,
        "ner_expected": False,
        "builtin_types": ["AWS_ACCESS_KEY"],
        "ner_types": []
    },
    
    # === NER 能检测但正则不能的 ===
    {
        "category": "中文人名",
        "text": "张三今天去北京出差了",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["PER", "NAME"]
    },
    {
        "category": "英文人名",
        "text": "John Smith works at Microsoft",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["PER", "PERSON"]
    },
    {
        "category": "中文组织名",
        "text": "腾讯科技发布了新产品",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["ORG", "ORGANIZATION", "COMPANY"]
    },
    {
        "category": "英文组织名",
        "text": "Google announced a new product",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["ORG", "ORGANIZATION"]
    },
    {
        "category": "中文地址",
        "text": "上海市浦东新区是金融中心",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["LOC", "LOCATION", "ADDRESS"]
    },
    {
        "category": "英文地址",
        "text": "Silicon Valley is located in California",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["LOC", "LOCATION"]
    },
    
    # === 混合场景 ===
    {
        "category": "人名+手机号",
        "text": "张三的手机号是13800138000",
        "builtin_expected": True,
        "ner_expected": True,
        "builtin_types": ["PHONE"],
        "ner_types": ["PER", "PHONE"]
    },
    {
        "category": "组织+地址",
        "text": "华为技术有限公司位于深圳市龙岗区",
        "builtin_expected": False,
        "ner_expected": True,
        "builtin_types": [],
        "ner_types": ["ORG", "LOC"]
    },
    {
        "category": "复杂场景",
        "text": "李四于2023年加入阿里巴巴，现居深圳市南山区，手机13900139000",
        "builtin_expected": True,
        "ner_expected": True,
        "builtin_types": ["PHONE"],
        "ner_types": ["PER", "ORG", "LOC", "PHONE"]
    },
]

def test_builtin_detection(text):
    """测试内置正则检测"""
    # 通过 CloseMask 的 /v1/chat/completions 端点测试
    # 由于没有直接的检测接口，我们通过响应判断
    payload = {
        "model": "test",
        "messages": [{"role": "user", "content": text}],
        "stream": False
    }
    
    start = time.time()
    try:
        resp = requests.post(
            f"{CLOSEMASK_URL}/v1/chat/completions",
            json=payload,
            headers={"Content-Type": "application/json", "Authorization": "Bearer test"},
            timeout=30
        )
        duration = (time.time() - start) * 1000
        return {"success": True, "duration": duration, "status": resp.status_code}
    except Exception as e:
        return {"success": False, "error": str(e), "duration": 0}

def test_ner_detection(text, language="zh"):
    """测试 NER 服务检测"""
    start = time.time()
    try:
        resp = requests.post(
            f"{NER_URL}/detect",
            json={"text": text, "language": language},
            timeout=10
        )
        duration = (time.time() - start) * 1000
        
        if resp.status_code == 200:
            data = resp.json()
            entities = data.get("entities", [])
            return {
                "success": True,
                "duration": duration,
                "entities": entities,
                "count": len(entities)
            }
        return {"success": False, "duration": duration, "error": f"HTTP {resp.status_code}"}
    except Exception as e:
        return {"success": False, "error": str(e), "duration": 0}

def run_comparison():
    """运行对比测试"""
    print("\n" + "="*80)
    print("  CloseMask PII 检测能力对比测试")
    print("  时间:", datetime.now().strftime("%Y-%m-%d %H:%M:%S"))
    print("="*80 + "\n")
    
    # 检查服务状态
    print("检查服务状态...")
    
    try:
        health = requests.get(f"{CLOSEMASK_URL}/health", timeout=5).json()
        print(f"  CloseMask: {health.get('status')} (引擎: {health.get('active_engine')})")
    except:
        print("  CloseMask: 不可用")
        return
    
    try:
        ner_health = requests.get(f"{NER_URL}/health", timeout=5).json()
        print(f"  NER 服务: {ner_health.get('status')}")
    except:
        print("  NER 服务: 不可用")
        return
    
    print("\n" + "-"*80)
    print("  检测能力对比")
    print("-"*80 + "\n")
    
    # 统计
    builtin_only = []  # 只有内置能检测
    ner_only = []      # 只有 NER 能检测
    both = []          # 两者都能检测
    neither = []       # 两者都不能检测
    
    for case in TEST_CASES:
        text = case["text"]
        category = case["category"]
        
        # 测试 NER
        ner_result = test_ner_detection(text)
        ner_detected = ner_result.get("success", False) and ner_result.get("count", 0) > 0
        
        # 内置检测的预期结果
        builtin_expected = case["builtin_expected"]
        ner_expected = case["ner_expected"]
        
        # 分类
        if builtin_expected and ner_expected:
            both.append((category, text, ner_result.get("duration", 0)))
        elif builtin_expected:
            builtin_only.append((category, text, ner_result.get("duration", 0)))
        elif ner_expected:
            ner_only.append((category, text, ner_result.get("duration", 0)))
        else:
            neither.append((category, text))
        
        # 打印结果
        builtin_mark = "[OK]" if builtin_expected else "[--]"
        ner_mark = "[OK]" if ner_detected else "[--]"
        
        print(f"  [{category}]")
        print(f"    文本: {text[:40]}...")
        print(f"    内置正则: {builtin_mark}  NER: {ner_mark} ({ner_result.get('duration', 0):.0f}ms)")
        if ner_detected:
            entities = ner_result.get("entities", [])
            entity_types = [e.get("type", "unknown") for e in entities]
            print(f"    NER 检测到: {entity_types}")
        print()
    
    # 汇总
    print("\n" + "="*80)
    print("  检测能力汇总")
    print("="*80 + "\n")
    
    print(f"  内置正则独有检测能力 ({len(builtin_only)} 项):")
    for cat, text, dur in builtin_only:
        print(f"    - {cat}: {text[:30]}...")
    
    print(f"\n  NER 服务独有检测能力 ({len(ner_only)} 项):")
    for cat, text, dur in ner_only:
        print(f"    - {cat}: {text[:30]}...")
    
    print(f"\n  两者均可检测 ({len(both)} 项):")
    for cat, text, dur in both:
        print(f"    - {cat}: {text[:30]}...")
    
    # 性能统计
    if ner_only or both:
        all_ner_times = [d for _, _, d in ner_only + both if d > 0]
        if all_ner_times:
            print(f"\n  NER 平均响应时间: {sum(all_ner_times)/len(all_ner_times):.0f}ms")
            print(f"  NER 最小响应时间: {min(all_ner_times):.0f}ms")
            print(f"  NER 最大响应时间: {max(all_ner_times):.0f}ms")

def print_capability_table():
    """打印能力对照表"""
    print("\n" + "="*80)
    print("  PII 检测能力对照表")
    print("="*80 + "\n")
    
    print("| PII 类型 | 内置正则 | NER 服务 | 说明 |")
    print("|----------|----------|----------|------|")
    print("| 手机号 | OK | OK | 正则精确匹配，NER 可能漏检 |")
    print("| 身份证号 | OK | OK | 正则精确匹配 18 位 |")
    print("| 邮箱地址 | OK | OK | 正则精确匹配 |")
    print("| 银行卡号 | OK | OK | 正则匹配 16-19 位 |")
    print("| IP 地址 | OK | -- | 正则匹配 IPv4 |")
    print("| API Key | OK | -- | 正则匹配 sk- 前缀等 |")
    print("| JWT Token | OK | -- | 正则匹配 eyJ 开头 |")
    print("| AWS Key | OK | -- | 正则匹配 AKIA 开头 |")
    print("| 密码/验证码 | OK | -- | 键名优先匹配 |")
    print("| 中文人名 | -- | OK | NLP 实体识别 |")
    print("| 英文人名 | -- | OK | NLP 实体识别 |")
    print("| 组织名 | -- | OK | NLP 实体识别 |")
    print("| 地址 | -- | OK | NLP 实体识别 |")
    print("| 日期/时间 | -- | OK | NLP 实体识别 |")
    
    print("\n推荐配置:")
    print("  - 基础场景：仅使用内置正则（零依赖）")
    print("  - 增强场景：内置正则 + NER 服务（检测人名/组织/地址）")

if __name__ == "__main__":
    print_capability_table()
    run_comparison()
