"""
综合测试脚本 - 测试所有场景
"""

import requests
import json
import time
from pathlib import Path

# ==================== 配置 ====================
PROXY_URL = "http://localhost:8846/v1/chat/completions"
ONEAIFW_URL = "http://localhost:8845"

def check_health():
    """健康检查"""
    try:
        resp = requests.get(f"{PROXY_URL.replace('/v1/chat/completions', '')}/health", timeout=5)
        return resp.status_code == 200
    except:
        return False

def test_file(file_path, name):
    """测试单个文件"""
    print(f"\n{'='*80}")
    print(f"测试: {name}")
    print(f"{'='*80}")
    
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
        
        print(f"文件长度: {len(content)} 字符")
        
        # 统计敏感信息
        pii_count = count_pii_types(content)
        print(f"\n敏感信息统计:")
        total_pii = 0
        for pii_type, count in pii_count.items():
            print(f"  {pii_type}: {count} 个")
            total_pii += count
        print(f"  总计: {total_pii} 个")
        
        # 发送到代理
        start_time = time.time()
        
        messages = [
            {"role": "system", "content": "你是一个测试助手，分析文本中的敏感信息。"},
            {"role": "user", "content": content}
        ]
        
        response = call_proxy(messages)
        
        elapsed = time.time() - start_time
        
        print(f"\n测试结果:")
        print(f"  状态: {'[OK] 成功' if response else '[FAIL] 失败'}")
        print(f"  处理时间: {elapsed:.2f} 秒")
        
        return {
            "name": name,
            "length": len(content),
            "pii_count": total_pii,
            "duration": elapsed,
            "status": "success" if response else "failed"
        }
        
    except Exception as e:
        print(f"[ERROR] 测试失败: {e}")
        return {
            "name": name,
            "status": "error",
            "error": str(e)
        }

def call_proxy(messages):
    """调用代理"""
    try:
        payload = {
            "model": "gpt-3.5-turbo",
            "messages": messages,
            "stream": False
        }
        resp = requests.post(PROXY_URL, json=payload, timeout=60)
        return resp.json()
    except Exception as e:
        print(f"Proxy error: {e}")
        return None

def count_pii_types(text):
    """统计敏感信息类型"""
    import re
    
    pii_types = {
        "身份证": len(re.findall(r'\d{17}[\dXx]', text)),
        "手机号": len(re.findall(r'1[3-9]\d{9}', text)),
        "银行卡": len(re.findall(r'\d{16,19}', text)),
        "邮箱": len(re.findall(r'[\w\.-]+@[\w\.-]+\.\w+', text)),
        "IP地址": len(re.findall(r'\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}', text)),
        "验证码": len(re.findall(r'\b\d{6}\b', text)),
        "API Key": len(re.findall(r'sk-[a-zA-Z0-9]{32,}', text)),
        "Access Key": len(re.findall(r'LTAI[a-zA-Z0-9]{20,}', text)),
        "Secret Key": len(re.findall(r'[a-zA-Z0-9/+]{40,}', text)),
        "UUID": len(re.findall(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', text, re.I)),
        "Bearer Token": len(re.findall(r'Bearer [a-zA-Z0-9\._-]+', text)),
        "证书私钥": len(re.findall(r'-----BEGIN.*PRIVATE KEY-----', text)),
        "JWT Token": len(re.findall(r'eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+', text)),
    }
    
    # 移除数量为0的类型
    return {k: v for k, v in pii_types.items() if v > 0}

def main():
    """主函数"""
    print("\n" + "="*80)
    print("综合测试 - 所有场景")
    print("="*80)
    
    # 检查服务状态
    if not check_health():
        print("\n[ERROR] 错误：代理服务未启动")
        print("请先启动代理服务：")
        print("  ./proxy.exe")
        print("  python aifw_v2.py")
        print("  python mock_llm_new.py")
        return
    
    print("\n[OK] 服务状态正常，开始测试...")
    
    # 测试文件列表
    scenarios_dir = Path(__file__).parent / 'scenarios'
    test_files = [
        (scenarios_dir / "test_real_conversation.txt", "真实对话场景"),
        (scenarios_dir / "test_hidden_pii_scenarios.txt", "隐含敏感数据场景"),
    ]
    
    results = []
    for file_path, name in test_files:
        if file_path.exists():
            result = test_file(file_path, name)
            results.append(result)
        else:
            print(f"\n[WARN] 文件不存在: {file_name}")
    
    # 总结
    print(f"\n{'='*80}")
    print("测试总结")
    print(f"{'='*80}")
    
    total = len(results)
    success = sum(1 for r in results if r.get('status') == 'success')
    failed = total - success
    avg_time = sum(r.get('duration', 0) for r in results) / total if total > 0 else 0
    total_pii = sum(r.get('pii_count', 0) for r in results)
    
    print(f"\n总测试数: {total}")
    print(f"成功: {success} [OK]")
    print(f"失败: {failed} [FAIL]")
    print(f"通过率: {success/total*100:.1f}%")
    print(f"敏感信息总数: {total_pii}")
    print(f"平均处理时间: {avg_time:.2f} 秒")
    
    # 详细结果
    print(f"\n{'='*80}")
    print("详细结果")
    print(f"{'='*80}")
    
    for result in results:
        status_icon = "[OK]" if result.get('status') == 'success' else "[FAIL]"
        print(f"\n{status_icon} {result.get('name')}")
        if result.get('length'):
            print(f"   文件长度: {result['length']} 字符")
        if result.get('pii_count'):
            print(f"   敏感信息: {result['pii_count']} 个")
        if result.get('duration'):
            print(f"   处理时间: {result['duration']:.2f}s")
        if result.get('error'):
            print(f"   错误: {result['error']}")
    
    print(f"\n{'='*80}")
    print("测试完成！")
    print(f"{'='*80}")

if __name__ == "__main__":
    main()
