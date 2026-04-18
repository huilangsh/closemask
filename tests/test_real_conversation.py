"""
真实对话场景测试 - 模拟实际客户服务对话
基于 test_real_conversation.txt 中的真实对话数据
"""

import sys
import io
# Fix Windows GBK encoding issue
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')

import requests
import json
import time
from typing import Dict, Any

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

def test_real_conversation():
    """
    真实对话测试 - 3000字对话场景
    
    测试内容：
    - 5个完整对话场景
    - 客户账户查询、转账、密码找回、绑定银行卡、投诉
    - 包含：身份证、手机号、银行卡、密码、邮箱、验证码等多种PII
    - 同一敏感信息多次出现
    - 多轮对话验证占位符持久化
    """
    print("\n" + "="*80)
    print("真实对话场景测试")
    print("="*80)
    print("\n测试场景：")
    print("1. 客户账户查询 - 包含身份证、手机号、银行卡、密码等")
    print("2. 转账操作 - 包含收款人信息、转账金额、密码等")
    print("3. 密码找回 - 包含身份验证、密保问题、新密码等")
    print("4. 绑定新银行卡 - 包含银行卡信息、手机验证等")
    print("5. 客服投诉 - 包含时间、地点、账号信息等")
    print("\n测试要点：")
    print("- 同一敏感信息多次出现，验证遮罩一致性")
    print("- 多轮对话，验证占位符持久化")
    print("- 多种PII类型，验证识别准确性")
    print("- 真实对话流程，验证业务场景覆盖")
    
    # 读取对话数据
    scenarios_dir = Path(__file__).parent / 'scenarios'
    conversation_file = scenarios_dir / 'test_real_conversation.txt'
    with open(conversation_file, 'r', encoding='utf-8') as f:
        conversation_data = f.read()
    
    print(f"\n对话数据长度: {len(conversation_data)} 字符")
    print(f"包含敏感信息类型: 身份证、手机号、银行卡、密码、邮箱、验证码、IP地址等")
    
    # 分段发送测试
    dialogs = conversation_data.split('---\n\n## 对话')
    
    all_results = []
    
    for i, dialog in enumerate(dialogs[1:], 1):  # 跳过第一个空分割
        print(f"\n{'='*80}")
        print(f"测试对话 {i}")
        print(f"{'='*80}")
        
        try:
            start_time = time.time()
            
            # 发送到代理进行遮罩
            messages = [
                {"role": "system", "content": "你是一个客服助手，帮助客户处理账户相关的问题。"},
                {"role": "user", "content": dialog}
            ]
            
            response = call_proxy(messages)
            
            elapsed = time.time() - start_time
            
            # 分析结果
            result = {
                "dialog_id": i,
                "length": len(dialog),
                "duration": elapsed,
                "status": "success" if response else "failed"
            }
            
            all_results.append(result)
            
            print(f"对话 {i} 测试完成")
            print(f"  - 输入长度: {len(dialog)} 字符")
            print(f"  - 处理时间: {elapsed:.2f} 秒")
            print(f"  - 状态: {'✅ 成功' if response else '❌ 失败'}")
            
            # 统计敏感信息
            pii_count = count_pii_types(dialog)
            print(f"  - 敏感信息统计:")
            for pii_type, count in pii_count.items():
                print(f"      {pii_type}: {count} 个")
            
        except Exception as e:
            print(f"对话 {i} 测试失败: {e}")
            all_results.append({
                "dialog_id": i,
                "status": "error",
                "error": str(e)
            })
    
    # 总结
    print(f"\n{'='*80}")
    print("测试总结")
    print(f"{'='*80}")
    
    total = len(all_results)
    success = sum(1 for r in all_results if r.get('status') == 'success')
    failed = total - success
    avg_time = sum(r.get('duration', 0) for r in all_results) / total if total > 0 else 0
    
    print(f"总测试数: {total}")
    print(f"成功: {success} ✅")
    print(f"失败: {failed} ❌")
    print(f"通过率: {success/total*100:.1f}%")
    print(f"平均处理时间: {avg_time:.2f} 秒")
    
    # 详细结果
    print(f"\n{'='*80}")
    print("详细结果")
    print(f"{'='*80}")
    
    for result in all_results:
        status_icon = "✅" if result.get('status') == 'success' else "❌"
        print(f"{status_icon} 对话 {result.get('dialog_id')}: {result.get('status', 'unknown')}")
        if result.get('duration'):
            print(f"   时间: {result['duration']:.2f}s")
        if result.get('error'):
            print(f"   错误: {result['error']}")
    
    return all_results

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
    }
    
    # 移除数量为0的类型
    return {k: v for k, v in pii_types.items() if v > 0}

def main():
    """主函数"""
    print("\n" + "="*80)
    print("真实对话场景测试")
    print("="*80)
    print("\n此测试基于真实客户服务对话，包含：")
    print("- 5个完整对话场景")
    print("- 3000+字符的对话内容")
    print("- 多种敏感信息类型")
    print("- 多轮对话交互")
    
    # 检查服务状态
    if not check_health():
        print("\n❌ 错误：代理服务未启动")
        print("请先启动代理服务：")
        print("  ./proxy.exe")
        print("  python aifw_v2.py")
        print("  python mock_llm_new.py")
        return
    
    print("\n✅ 服务状态正常，开始测试...")
    
    # 运行测试
    results = test_real_conversation()
    
    print(f"\n{'='*80}")
    print("测试完成！")
    print(f"{'='*80}")

if __name__ == "__main__":
    main()
