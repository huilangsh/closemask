"""
NER 检测器 - 基于 Transformers Pipeline
支持中英文 NER 模型
"""
import os
import re
import json
import logging
from typing import List, Dict, Optional
from pathlib import Path

from transformers import pipeline, AutoTokenizer, AutoModelForTokenClassification

logger = logging.getLogger(__name__)


class NEREntity:
    """NER 实体"""
    def __init__(self, entity_type: str, value: str, start: int, end: int, score: float = 1.0):
        self.type = entity_type
        self.value = value
        self.start = start
        self.end = end
        self.score = score

    def to_dict(self) -> Dict:
        return {
            "type": self.type,
            "value": self.value,
            "start": self.start,
            "end": self.end,
            "score": self.score
        }


class NERDetector:
    """NER 检测器 - 基于 Transformers Pipeline"""
    
    # NER 标签到 PII 类型的映射
    LABEL_MAP = {
        # 中文 NER 标签 (ckiplab/bert-tiny-chinese-ner)
        "PER": "PERSON",
        "LOC": "LOCATION",
        "ORG": "ORGANIZATION",
        "GPE": "LOCATION",
        "DATE": "DATE",
        "TIME": "TIME",
        # 英文 NER 标签 (dslim/distilbert-NER)
        "B-PER": "PERSON",
        "I-PER": "PERSON",
        "B-LOC": "LOCATION",
        "I-LOC": "LOCATION",
        "B-ORG": "ORGANIZATION",
        "I-ORG": "ORGANIZATION",
        "B-GPE": "LOCATION",
        "I-GPE": "LOCATION",
        "B-DATE": "DATE",
        "I-DATE": "DATE",
        "B-TIME": "TIME",
        "I-TIME": "TIME",
        "B-MONEY": "MONEY",
        "I-MONEY": "MONEY",
        "B-PERCENT": "PERCENT",
        "I-PERCENT": "PERCENT",
        "PERSON": "PERSON",
        "LOCATION": "LOCATION",
        "ORGANIZATION": "ORGANIZATION",
    }
    
    # 日期/时间正则表达式
    DATETIME_PATTERNS = {
        # 中文日期格式
        "zh_date": [
            r"\d{4}年\d{1,2}月\d{1,2}日",  # 2024年1月15日
            r"\d{4}年\d{1,2}月",  # 2024年1月
            r"\d{1,2}月\d{1,2}日",  # 1月15日
        ],
        # 中文时间格式
        "zh_time": [
            r"下午\d{1,2}点",  # 下午3点
            r"上午\d{1,2}点",  # 上午9点
            r"\d{1,2}点\d{1,2}分",  # 3点30分
            r"\d{1,2}:\d{2}",  # 15:30
        ],
        # 英文日期格式
        "en_date": [
            r"\b(?:January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b",  # January 15, 2024
            r"\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2},?\s+\d{4}\b",  # Jan 15, 2024
            r"\b\d{4}-\d{2}-\d{2}\b",  # 2024-01-15
            r"\b\d{1,2}/\d{1,2}/\d{4}\b",  # 01/15/2024
        ],
        # 英文时间格式
        "en_time": [
            r"\b\d{1,2}:\d{2}\s*(?:AM|PM|am|pm)?\b",  # 3:30 PM
            r"\b\d{1,2}\s*(?:AM|PM|am|pm)\b",  # 3 PM
        ],
    }
    
    def __init__(self, model_dir: str = "./models"):
        self.model_dir = Path(model_dir)
        self.model_dir.mkdir(parents=True, exist_ok=True)
        
        self.pipelines: Dict[str, any] = {}
        self.model_names: Dict[str, str] = {}
        
    def load_model(self, language: str, model_name: str) -> bool:
        """加载指定语言的 NER 模型"""
        try:
            save_path = self.model_dir / language
            
            # 检查是否已下载
            if not (save_path / "config.json").exists():
                logger.info(f"Model not found at {save_path}, downloading...")
                if not self._download_model(language, model_name):
                    return False
            
            # 加载 pipeline
            logger.info(f"Loading NER pipeline for {language} from {save_path}")
            
            tokenizer = AutoTokenizer.from_pretrained(str(save_path))
            model = AutoModelForTokenClassification.from_pretrained(str(save_path))
            
            ner_pipeline = pipeline(
                "ner",
                model=model,
                tokenizer=tokenizer,
                aggregation_strategy="simple"
            )
            
            self.pipelines[language] = ner_pipeline
            self.model_names[language] = model_name
            
            logger.info(f"Loaded NER model for {language}: {model_name}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to load NER model for {language}: {e}")
            import traceback
            logger.error(traceback.format_exc())
            return False
    
    def _download_model(self, language: str, model_name: str) -> bool:
        """从 HuggingFace 下载模型"""
        try:
            save_path = self.model_dir / language
            save_path.mkdir(parents=True, exist_ok=True)
            
            logger.info(f"Downloading model {model_name}...")
            
            # 下载 tokenizer
            tokenizer = AutoTokenizer.from_pretrained(model_name)
            tokenizer.save_pretrained(str(save_path))
            
            # 下载模型
            model = AutoModelForTokenClassification.from_pretrained(model_name)
            model.save_pretrained(str(save_path))
            
            logger.info(f"Model saved to {save_path}")
            return True
            
        except Exception as e:
            logger.error(f"Failed to download model: {e}")
            import traceback
            logger.error(traceback.format_exc())
            return False
    
    def detect(self, text: str, language: str = "zh") -> List[NEREntity]:
        """检测文本中的 NER 实体"""
        if language not in self.pipelines:
            logger.warning(f"NER model not loaded for {language}")
            return []
        
        try:
            ner_pipeline = self.pipelines[language]
            results = ner_pipeline(text)
            
            entities = []
            for r in results:
                entity_type = r.get("entity_group", r.get("entity", "UNKNOWN"))
                # 移除 B-/I- 前缀
                if entity_type.startswith("B-") or entity_type.startswith("I-"):
                    entity_type = entity_type[2:]
                
                mapped_type = self.LABEL_MAP.get(entity_type, entity_type)
                
                # 使用原始文本切片，而不是 tokenizer 的 word（可能有空格）
                start = r.get("start", 0)
                end = r.get("end", 0)
                value = text[start:end] if start < end else r.get("word", "")
                
                entity = NEREntity(
                    entity_type=mapped_type,
                    value=value,
                    start=start,
                    end=end,
                    score=r.get("score", 1.0)
                )
                entities.append(entity)
            
            # 补充日期/时间检测
            datetime_entities = self._detect_datetime(text, language)
            entities.extend(datetime_entities)
            
            # 去重（按位置）
            seen = set()
            unique_entities = []
            for e in entities:
                key = (e.start, e.end)
                if key not in seen:
                    seen.add(key)
                    unique_entities.append(e)
            
            return unique_entities
            
        except Exception as e:
            logger.error(f"NER detection failed: {e}")
            return []
    
    def _detect_datetime(self, text: str, language: str) -> List[NEREntity]:
        """使用正则表达式检测日期/时间"""
        entities = []
        
        # 根据语言选择模式
        if language == "zh":
            date_patterns = self.DATETIME_PATTERNS["zh_date"]
            time_patterns = self.DATETIME_PATTERNS["zh_time"]
        else:
            date_patterns = self.DATETIME_PATTERNS["en_date"]
            time_patterns = self.DATETIME_PATTERNS["en_time"]
        
        # 检测日期
        for pattern in date_patterns:
            for match in re.finditer(pattern, text):
                entity = NEREntity(
                    entity_type="DATE",
                    value=match.group(),
                    start=match.start(),
                    end=match.end(),
                    score=1.0
                )
                entities.append(entity)
        
        # 检测时间
        for pattern in time_patterns:
            for match in re.finditer(pattern, text):
                entity = NEREntity(
                    entity_type="TIME",
                    value=match.group(),
                    start=match.start(),
                    end=match.end(),
                    score=1.0
                )
                entities.append(entity)
        
        return entities
    
    def is_loaded(self, language: str) -> bool:
        """检查模型是否已加载"""
        return language in self.pipelines
    
    def get_loaded_languages(self) -> List[str]:
        """获取已加载的语言列表"""
        return list(self.pipelines.keys())
