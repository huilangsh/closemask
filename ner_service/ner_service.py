"""
NER 服务 - FastAPI 服务
提供 HTTP 接口供 CloseMask 主服务调用
"""
import os
import sys
import json
import logging
import signal
import asyncio
from typing import List, Optional
from pathlib import Path
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field

# 添加当前目录到 path
sys.path.insert(0, str(Path(__file__).parent))

from ner_detector import NERDetector, NEREntity

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.StreamHandler(),
        logging.FileHandler('logs/ner_service.log', encoding='utf-8')
    ]
)
logger = logging.getLogger(__name__)

# 确保日志目录存在
Path('logs').mkdir(exist_ok=True)

# 全局 NER 检测器
detector: Optional[NERDetector] = None

# 配置
CONFIG = {
    "port": int(os.getenv("NER_PORT", "8847")),
    "model_dir": os.getenv("NER_MODEL_DIR", "./models"),
    "models": {
        "zh": os.getenv("NER_MODEL_ZH", "gyr66/bert-base-chinese-finetuned-ner"),
        "en": os.getenv("NER_MODEL_EN", "elastic/distilbert-base-cased-finetuned-conll03-english")
    }
}


# Lifespan 事件处理器（替代弃用的 on_event）
@asynccontextmanager
async def lifespan(app: FastAPI):
    """服务生命周期管理"""
    global detector
    
    # 启动时初始化
    logger.info(f"Starting NER Service on port {CONFIG['port']}")
    logger.info(f"Model directory: {CONFIG['model_dir']}")
    
    detector = NERDetector(model_dir=CONFIG['model_dir'])
    
    # 尝试加载模型（可选，首次请求时加载也可以）
    for lang, model_name in CONFIG['models'].items():
        try:
            logger.info(f"Pre-loading model for {lang}: {model_name}")
            detector.load_model(lang, model_name)
        except Exception as e:
            logger.warning(f"Failed to pre-load model for {lang}: {e}")
            logger.info(f"Model will be loaded on first request")
    
    yield  # 服务运行中
    
    # 关闭时清理
    logger.info("Shutting down NER Service")


# FastAPI 应用
app = FastAPI(
    title="NER Service",
    description="NER 服务 - 为 CloseMask 提供语义 PII 检测",
    version="1.0.0",
    lifespan=lifespan
)


# 请求/响应模型
class DetectRequest(BaseModel):
    text: str = Field(..., description="待检测文本")
    language: str = Field(default="zh", description="语言: zh 或 en")


class Entity(BaseModel):
    type: str = Field(..., description="实体类型")
    value: str = Field(..., description="实体值")
    start: int = Field(..., description="起始位置")
    end: int = Field(..., description="结束位置")
    score: float = Field(default=1.0, description="置信度")


class DetectResponse(BaseModel):
    entities: List[Entity] = Field(default_factory=list, description="检测到的实体")


class HealthResponse(BaseModel):
    status: str
    models: List[str]
    uptime: float


# 启动时间
import time
start_time = time.time()


@app.post("/detect", response_model=DetectResponse)
async def detect(req: DetectRequest):
    """
    检测文本中的 NER 实体
    
    - **text**: 待检测的文本
    - **language**: 语言 (zh/en)
    """
    if detector is None:
        raise HTTPException(status_code=503, detail="NER detector not initialized")
    
    if not req.text:
        return DetectResponse(entities=[])
    
    # 如果模型未加载，尝试加载
    if not detector.is_loaded(req.language):
        model_name = CONFIG['models'].get(req.language)
        if model_name:
            logger.info(f"Loading model for {req.language}: {model_name}")
            if not detector.load_model(req.language, model_name):
                raise HTTPException(
                    status_code=503,
                    detail=f"Failed to load NER model for {req.language}"
                )
        else:
            raise HTTPException(
                status_code=400,
                detail=f"Unsupported language: {req.language}"
            )
    
    # 执行检测
    try:
        entities = detector.detect(req.text, req.language)
        return DetectResponse(
            entities=[
                Entity(
                    type=e.type,
                    value=e.value,
                    start=e.start,
                    end=e.end,
                    score=e.score
                )
                for e in entities
            ]
        )
    except Exception as e:
        logger.error(f"Detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/health", response_model=HealthResponse)
async def health():
    """健康检查"""
    return HealthResponse(
        status="ok" if detector is not None else "initializing",
        models=detector.get_loaded_languages() if detector else [],
        uptime=time.time() - start_time
    )


@app.get("/models")
async def list_models():
    """列出已加载的模型"""
    if detector is None:
        return {"models": []}
    
    return {
        "loaded": detector.get_loaded_languages(),
        "available": CONFIG['models']
    }


@app.post("/load/{language}")
async def load_model(language: str):
    """手动加载指定语言的模型"""
    if detector is None:
        raise HTTPException(status_code=503, detail="NER detector not initialized")
    
    model_name = CONFIG['models'].get(language)
    if not model_name:
        raise HTTPException(status_code=400, detail=f"Unknown language: {language}")
    
    if detector.is_loaded(language):
        return {"status": "already_loaded", "language": language}
    
    success = detector.load_model(language, model_name)
    if success:
        return {"status": "loaded", "language": language, "model": model_name}
    else:
        raise HTTPException(
            status_code=500,
            detail=f"Failed to load model for {language}"
        )


def signal_handler(sig, frame):
    """信号处理"""
    logger.info(f"Received signal {sig}, shutting down...")
    sys.exit(0)


if __name__ == "__main__":
    import uvicorn
    
    # 注册信号处理
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    # 启动服务
    uvicorn.run(
        app,
        host="127.0.0.1",
        port=CONFIG['port'],
        log_level="info"
    )
