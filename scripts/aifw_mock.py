#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
OneAIFW 简化版 PII 遮罩服务 V2
支持基本的 PII 遮罩和还原
"""

import json
import re
import uuid
from flask import Flask, request, jsonify
from datetime import datetime

app = Flask(__name__)

# PII 占位符存储
placeholder_store = {}

# PII 模式 - 只匹配数字 PII
PII_PATTERNS = [
    # 手机号
    (r'1[3-9]\d{9}', 'PHONE'),
    # 身份证
    (r'[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]', 'ID_CARD'),
    # 邮箱
    (r'[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}', 'EMAIL'),
]

@app.route('/api/health', methods=['GET'])
def health_check():
    return jsonify({
        'status': 'ok',
        'model': 'simple-pii-masker-v2',
        'timestamp': datetime.now().isoformat()
    })

@app.route('/api/mask_text', methods=['POST'])
def mask_text():
    data = request.get_json()
    if not data or 'text' not in data:
        return jsonify({'error': 'Missing text parameter'}), 400

    text = data['text']
    language = data.get('language', 'zh')
    placeholders = []
    masked_text = text

    # 按顺序替换 PII
    for pattern, pii_type in PII_PATTERNS:
        matches = list(re.finditer(pattern, masked_text))
        # 从后往前替换，避免索引偏移
        for match in reversed(matches):
            original = match.group()
            placeholder_id = str(uuid.uuid4())[:8]

            # 生成占位符
            placeholder = f'__{pii_type}_{placeholder_id}__'

            # 存储映射
            placeholder_store[placeholder] = original
            placeholders.append({
                'placeholder': placeholder,
                'value': original,
                'type': pii_type,
                'start': match.start(),
                'end': match.end()
            })

            # 替换文本
            masked_text = masked_text[:match.start()] + placeholder + masked_text[match.end():]

    # 返回与 Go handler 期望的格式一致的响应
    mask_meta_json = json.dumps({"pii": placeholders}, ensure_ascii=False)
    return jsonify({
        'output': {
            'text': masked_text,
            'maskMeta': mask_meta_json
        }
    })

@app.route('/api/restore_text', methods=['POST'])
def restore_text():
    data = request.get_json()
    if not data or 'text' not in data:
        return jsonify({'error': 'Missing text parameter'}), 400

    text = data['text']

    # 如果提供了 maskMeta，还原占位符
    mask_meta = data.get('maskMeta', '')
    restored_text = text

    if mask_meta:
        try:
            meta = json.loads(mask_meta)
            pii_list = meta.get('pii', [])
            for pii in pii_list:
                placeholder = pii.get('placeholder', '')
                value = pii.get('value', '')
                if placeholder and value:
                    restored_text = restored_text.replace(placeholder, value)
        except:
            pass
    else:
        # 简单还原：遍历所有存储的占位符
        for placeholder, original in placeholder_store.items():
            restored_text = restored_text.replace(placeholder, original)

    return jsonify({
        'output': {
            'text': restored_text
        }
    })

if __name__ == '__main__':
    print("OneAIFW V2 Service starting on port 8845...")
    app.run(host='0.0.0.0', port=8845, debug=False)
