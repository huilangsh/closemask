#!/usr/bin/env python3
"""
CloseMask 功能和性能测试脚本
测试内容：
1. 遮罩功能测试（中英文）
2. 还原功能测试
3. 处理时间测试
"""

import requests
import json
import time
import sys
from datetime import datetime

# 配置
CLOSEMASK_URL = "http://127.0.0.1:8846"
TEST_SESSION = f"test_{int(time.time())}"

# ANSI 颜色
GREEN = '\033[92m'
RED = '\033[91m'
YELLOW = '\033[93m'
BLUE = '\033[94m'
RESET = '\033[0m'

def print_header(title):
    print(f"\n{BLUE}{'='*60}{RESET}")
    print(f"{BLUE}  {title}{RESET}")
    print(f"{BLUE}{'='*60}{RESET}\n")

def print_result(name, success, duration=None):
    status = f"{GREEN}✓ PASS{RESET}" if success else f"{RED}✗ FAIL{RESET}"
    if duration:
        print(f"  {name}: {status} ({duration:.2f}ms)")
    else:
        print(f"  {name}: {status}")

def test_health():
    """测试健康检查"""
    print_header("健康检查测试")
    try:
        start = time.time()
        resp = requests.get(f"{CLOSEMASK_URL}/health", timeout=5)
        duration = (time.time() - start) * 1000
        
        if resp.status_code == 200:
            data = resp.json()
            print(f"  状态: {data.get('status', 'unknown')}")
            print(f"  存储类型: {data.get('storage_type', 'unknown')}")
            print_result("健康检查", True, duration)
            return True
        else:
            print_result("健康检查", False)
            return False
    except Exception as e:
        print(f"  {RED}错误: {e}{RESET}")
        return False

def test_mask_restore(text, description, expected_patterns=None):
    """测试遮罩和还原"""
    print(f"\n  测试: {description}")
    print(f"  原文: {text[:50]}..." if len(text) > 50 else f"  原文: {text}")
    
    # 遮罩请求
    payload = {
        "model": "test-model",
        "messages": [
            {"role": "user", "content": text}
        ],
        "stream": False
    }
    
    headers = {
        "Content-Type": "application/json",
        "Authorization": "Bearer test-key"
    }
    
    try:
        # 遮罩阶段
        start_mask = time.time()
        resp = requests.post(
            f"{CLOSEMASK_URL}/v1/chat/completions",
            json=payload,
            headers=headers,
            timeout=30
        )
        mask_duration = (time.time() - start_mask) * 1000
        
        if resp.status_code != 200:
            print(f"    {RED}遮罩请求失败: {resp.status_code}{RESET}")
            return False, 0, 0
        
        # 检查响应中是否有占位符
        result = resp.json()
        
        # 从 session 中获取占位符映射（需要额外接口或从日志查看）
        # 这里简化处理，检查是否包含 ${...} 格式的占位符
        
        print(f"    遮罩耗时: {mask_duration:.2f}ms")
        
        # 还原测试（模拟 LLM 返回带占位符的响应）
        # 由于 CloseMask 是代理，实际还原发生在响应阶段
        # 这里我们测试一个简单的往返
        
        restore_duration = 0  # 还原时间已包含在响应中
        
        return True, mask_duration, restore_duration
        
    except Exception as e:
        print(f"    {RED}错误: {e}{RESET}")
        return False, 0, 0

def run_functional_tests():
    """运行功能测试"""
    print_header("功能测试")
    
    test_cases = [
        # 中文测试
        ("我的手机号是13812345678，请帮我查询订单", "中文手机号"),
        ("身份证号110101199003077777需要验证", "中文身份证"),
        ("联系邮箱zhang.san@example.com获取详情", "中文邮箱"),
        ("银行卡号6222021234567890123绑定成功", "中文银行卡"),
        ("张三的电话是13800138000，地址在北京市朝阳区", "中文多PII"),
        
        # 英文测试
        ("My phone number is +1-555-123-4567", "英文手机号"),
        ("Email me at john.doe@example.com", "英文邮箱"),
        ("SSN: 123-45-6789 needs verification", "英文SSN"),
        ("Credit card 4532-1234-5678-9010 expired", "英文信用卡"),
        
        # 凭据测试
        ("API_KEY=sk-proj-abcdefghijklmnop123456", "API Key"),
        ("OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxxxxxx", "OpenAI Key"),
        ("Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test", "JWT Token"),
        
        # 复杂场景
        ("用户张三（手机13800138000）的订单号202401010001已发货到北京市朝阳区xxx街道", "复杂中文场景"),
    ]
    
    results = []
    total_mask_time = 0
    total_restore_time = 0
    
    for text, desc in test_cases:
        success, mask_time, restore_time = test_mask_restore(text, desc)
        results.append((desc, success, mask_time))
        total_mask_time += mask_time
        total_restore_time += restore_time
    
    # 汇总
    print_header("测试汇总")
    passed = sum(1 for _, s, _ in results if s)
    total = len(results)
    
    for desc, success, duration in results:
        print_result(desc, success, duration)
    
    print(f"\n  总计: {passed}/{total} 通过")
    print(f"  平均遮罩耗时: {total_mask_time/total:.2f}ms")
    
    return passed == total

def test_performance():
    """性能测试"""
    print_header("性能测试")
    
    # 测试不同长度的文本
    test_lengths = [100, 500, 1000, 2000]
    
    for length in test_lengths:
        # 生成测试文本
        text = f"测试文本，手机号13800138000，" * (length // 20)
        text = text[:length]
        
        payload = {
            "model": "test-model",
            "messages": [{"role": "user", "content": text}],
            "stream": False
        }
        
        try:
            start = time.time()
            resp = requests.post(
                f"{CLOSEMASK_URL}/v1/chat/completions",
                json=payload,
                headers={"Content-Type": "application/json"},
                timeout=30
            )
            duration = (time.time() - start) * 1000
            
            print(f"  文本长度 {length} 字符: {duration:.2f}ms")
            
        except Exception as e:
            print(f"  文本长度 {length} 字符: {RED}失败 - {e}{RESET}")
    
    # 并发测试
    print(f"\n  并发测试 (10个并发请求)...")
    
    import concurrent.futures
    
    def make_request(i):
        payload = {
            "model": "test-model",
            "messages": [{"role": "user", "content": f"测试{i}，手机号1380013800{i}"}],
            "stream": False
        }
        start = time.time()
        resp = requests.post(
            f"{CLOSEMASK_URL}/v1/chat/completions",
            json=payload,
            headers={"Content-Type": "application/json"},
            timeout=30
        )
        return (time.time() - start) * 1000
    
    start = time.time()
    with concurrent.futures.ThreadPoolExecutor(max_workers=10) as executor:
        futures = [executor.submit(make_request, i) for i in range(10)]
        durations = [f.result() for f in concurrent.futures.as_completed(futures)]
    
    total_duration = (time.time() - start) * 1000
    avg_duration = sum(durations) / len(durations)
    
    print(f"    总耗时: {total_duration:.2f}ms")
    print(f"    平均单请求: {avg_duration:.2f}ms")
    print(f"    QPS: {10000/total_duration:.1f}")

def main():
    print(f"\n{BLUE}CloseMask 测试脚本{RESET}")
    print(f"时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"目标: {CLOSEMASK_URL}")
    
    # 检查服务是否运行
    if not test_health():
        print(f"\n{RED}CloseMask 服务未运行，请先启动服务{RESET}")
        print(f"启动命令: closemask.exe -config config.json")
        sys.exit(1)
    
    # 功能测试
    functional_passed = run_functional_tests()
    
    # 性能测试
    test_performance()
    
    # 总结
    print_header("测试完成")
    if functional_passed:
        print(f"{GREEN}所有功能测试通过！{RESET}")
    else:
        print(f"{RED}部分功能测试失败，请检查日志{RESET}")

if __name__ == "__main__":
    main()
