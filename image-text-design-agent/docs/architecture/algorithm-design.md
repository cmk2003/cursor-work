# 算法端技术方案设计

## 1. 技术栈选型

### 1.1 核心框架
- **Python 3.11+**：算法开发主语言
- **LangChain 0.1+**：LLM应用开发框架
- **LangGraph 0.0.25+**：构建状态化的Agent工作流
- **FastAPI**：高性能异步Web框架

### 1.2 AI模型
- **LLM**：GPT-4/Claude/文心一言（意图理解、文案生成）
- **Stable Diffusion XL**：图像生成主模型
- **ControlNet**：精确控制图像生成
- **CLIP**：图文匹配评估
- **PaddleOCR**：中文OCR识别

### 1.3 图像处理
- **OpenCV**：图像处理基础库
- **Pillow**：图像操作
- **Scikit-image**：高级图像处理
- **Albumentations**：图像增强

### 1.4 基础设施
- **Celery**：分布式任务队列
- **Redis**：任务队列后端
- **NVIDIA Triton**：模型推理服务器
- **MLflow**：模型版本管理

## 2. LangGraph工作流架构

### 2.1 核心概念设计

```python
# algorithm/langgraph/core/state.py
from typing import TypedDict, List, Dict, Any, Optional
from langgraph.graph import StateGraph

class DesignState(TypedDict):
    """设计生成的状态定义"""
    # 输入信息
    task_id: str
    prompt: str
    style: str
    size: tuple[int, int]
    reference_images: List[str]
    
    # 中间状态
    parsed_intent: Dict[str, Any]
    design_elements: List[Dict[str, Any]]
    generated_images: List[str]
    ocr_results: List[Dict[str, Any]]
    quality_scores: Dict[str, float]
    
    # 输出结果
    final_image: Optional[str]
    error: Optional[str]
    metadata: Dict[str, Any]

# algorithm/langgraph/core/graph.py
class DesignGenerationGraph:
    """图文设计生成工作流"""
    
    def __init__(self):
        self.graph = StateGraph(DesignState)
        self._build_graph()
    
    def _build_graph(self):
        # 添加节点
        self.graph.add_node("parse_intent", self.parse_intent_node)
        self.graph.add_node("generate_elements", self.generate_elements_node)
        self.graph.add_node("generate_image", self.generate_image_node)
        self.graph.add_node("ocr_check", self.ocr_check_node)
        self.graph.add_node("quality_assessment", self.quality_assessment_node)
        self.graph.add_node("refinement", self.refinement_node)
        self.graph.add_node("finalize", self.finalize_node)
        
        # 定义边
        self.graph.add_edge("parse_intent", "generate_elements")
        self.graph.add_edge("generate_elements", "generate_image")
        self.graph.add_edge("generate_image", "ocr_check")
        
        # 条件边
        self.graph.add_conditional_edges(
            "ocr_check",
            self.check_ocr_quality,
            {
                "pass": "quality_assessment",
                "fail": "refinement"
            }
        )
        
        self.graph.add_conditional_edges(
            "quality_assessment",
            self.check_quality,
            {
                "pass": "finalize",
                "fail": "refinement"
            }
        )
        
        self.graph.add_edge("refinement", "generate_image")
        
        # 设置入口和出口
        self.graph.set_entry_point("parse_intent")
        self.graph.set_finish_point("finalize")
```

### 2.2 核心节点实现

```python
# algorithm/langgraph/nodes/intent_parser.py
from langchain.chat_models import ChatOpenAI
from langchain.prompts import ChatPromptTemplate

class IntentParserNode:
    """意图解析节点"""
    
    def __init__(self):
        self.llm = ChatOpenAI(model="gpt-4", temperature=0)
        self.prompt = ChatPromptTemplate.from_messages([
            ("system", "你是一个专业的设计需求分析师，需要从用户的自然语言描述中提取设计要素。"),
            ("user", "{prompt}")
        ])
    
    async def run(self, state: DesignState) -> DesignState:
        # 构建提示词
        chain = self.prompt | self.llm | JsonOutputParser()
        
        # 解析用户意图
        parsed_intent = await chain.ainvoke({
            "prompt": state["prompt"]
        })
        
        # 提取设计要素
        design_elements = self._extract_elements(parsed_intent)
        
        state["parsed_intent"] = parsed_intent
        state["design_elements"] = design_elements
        
        return state
    
    def _extract_elements(self, intent: Dict) -> List[Dict]:
        """提取设计要素"""
        elements = []
        
        # 提取文本元素
        if "texts" in intent:
            for text in intent["texts"]:
                elements.append({
                    "type": "text",
                    "content": text["content"],
                    "style": text.get("style", {}),
                    "position": text.get("position", "auto")
                })
        
        # 提取图形元素
        if "graphics" in intent:
            for graphic in intent["graphics"]:
                elements.append({
                    "type": "graphic",
                    "shape": graphic["shape"],
                    "style": graphic.get("style", {}),
                    "position": graphic.get("position", "auto")
                })
        
        return elements

# algorithm/langgraph/nodes/image_generator.py
import torch
from diffusers import StableDiffusionXLPipeline, ControlNetModel

class ImageGeneratorNode:
    """图像生成节点"""
    
    def __init__(self):
        self.base_model = StableDiffusionXLPipeline.from_pretrained(
            "stabilityai/stable-diffusion-xl-base-1.0",
            torch_dtype=torch.float16,
            use_safetensors=True
        ).to("cuda")
        
        self.controlnet = ControlNetModel.from_pretrained(
            "diffusers/controlnet-canny-sdxl-1.0",
            torch_dtype=torch.float16
        )
    
    async def run(self, state: DesignState) -> DesignState:
        # 构建生成提示词
        prompt = self._build_prompt(state)
        
        # 生成布局控制图
        control_image = self._generate_layout(state["design_elements"])
        
        # 图像生成
        images = []
        for i in range(3):  # 生成多个候选
            image = self.base_model(
                prompt=prompt,
                negative_prompt=self._get_negative_prompt(state["style"]),
                image=control_image,
                num_inference_steps=30,
                guidance_scale=7.5,
                controlnet_conditioning_scale=0.5
            ).images[0]
            
            # 后处理：添加文字
            image_with_text = self._add_text_elements(image, state["design_elements"])
            
            images.append(image_with_text)
        
        state["generated_images"] = images
        return state
    
    def _build_prompt(self, state: DesignState) -> str:
        """构建SD提示词"""
        style_prompts = {
            "business": "professional, modern, clean, corporate",
            "cartoon": "colorful, playful, cartoon style, cute",
            "minimalist": "minimal, simple, clean, white space"
        }
        
        base_prompt = f"{state['parsed_intent'].get('theme', '')}, "
        base_prompt += style_prompts.get(state['style'], state['style'])
        base_prompt += ", high quality, 4k, detailed"
        
        return base_prompt
    
    def _add_text_elements(self, image, elements):
        """添加文字元素到图像"""
        from PIL import Image, ImageDraw, ImageFont
        
        img = Image.fromarray(image)
        draw = ImageDraw.Draw(img)
        
        for element in elements:
            if element["type"] == "text":
                # 智能字体选择
                font = self._select_font(element["style"])
                
                # 智能位置计算
                position = self._calculate_position(
                    img.size, 
                    element["content"], 
                    font,
                    element["position"]
                )
                
                # 绘制文字
                draw.text(
                    position,
                    element["content"],
                    font=font,
                    fill=element["style"].get("color", "black")
                )
        
        return np.array(img)

# algorithm/langgraph/nodes/ocr_checker.py
from paddleocr import PaddleOCR

class OCRCheckerNode:
    """OCR检查节点"""
    
    def __init__(self):
        self.ocr = PaddleOCR(use_angle_cls=True, lang='ch')
    
    async def run(self, state: DesignState) -> DesignState:
        ocr_results = []
        
        for idx, image_path in enumerate(state["generated_images"]):
            # 执行OCR
            result = self.ocr.ocr(image_path, cls=True)
            
            # 提取识别的文字
            detected_texts = []
            for line in result:
                for word_info in line:
                    detected_texts.append({
                        "text": word_info[1][0],
                        "confidence": word_info[1][1],
                        "position": word_info[0]
                    })
            
            # 验证文字正确性
            accuracy = self._verify_text_accuracy(
                detected_texts,
                state["design_elements"]
            )
            
            ocr_results.append({
                "image_index": idx,
                "detected_texts": detected_texts,
                "accuracy": accuracy,
                "errors": self._find_text_errors(detected_texts, state["design_elements"])
            })
        
        state["ocr_results"] = ocr_results
        return state
    
    def _verify_text_accuracy(self, detected_texts, expected_elements):
        """验证文字准确性"""
        expected_texts = [
            elem["content"] 
            for elem in expected_elements 
            if elem["type"] == "text"
        ]
        
        if not expected_texts:
            return 1.0
        
        correct_count = 0
        for expected in expected_texts:
            # 模糊匹配
            if any(self._fuzzy_match(expected, detected["text"]) 
                   for detected in detected_texts):
                correct_count += 1
        
        return correct_count / len(expected_texts)

# algorithm/langgraph/nodes/quality_assessor.py
import clip
import torch

class QualityAssessorNode:
    """质量评估节点"""
    
    def __init__(self):
        self.clip_model, self.clip_preprocess = clip.load("ViT-B/32", device="cuda")
        self.aesthetic_model = self._load_aesthetic_model()
    
    async def run(self, state: DesignState) -> DesignState:
        quality_scores = {}
        
        for idx, image_path in enumerate(state["generated_images"]):
            scores = {
                "clip_score": self._calculate_clip_score(image_path, state["prompt"]),
                "aesthetic_score": self._calculate_aesthetic_score(image_path),
                "layout_score": self._calculate_layout_score(image_path),
                "text_quality": state["ocr_results"][idx]["accuracy"]
            }
            
            # 综合评分
            scores["overall"] = self._calculate_overall_score(scores)
            quality_scores[f"image_{idx}"] = scores
        
        state["quality_scores"] = quality_scores
        
        # 选择最佳图像
        best_image_idx = max(
            range(len(state["generated_images"])),
            key=lambda i: quality_scores[f"image_{i}"]["overall"]
        )
        
        state["final_image"] = state["generated_images"][best_image_idx]
        
        return state
    
    def _calculate_clip_score(self, image_path, prompt):
        """计算CLIP相似度分数"""
        image = self.clip_preprocess(Image.open(image_path)).unsqueeze(0).to("cuda")
        text = clip.tokenize([prompt]).to("cuda")
        
        with torch.no_grad():
            image_features = self.clip_model.encode_image(image)
            text_features = self.clip_model.encode_text(text)
            
            # 归一化
            image_features /= image_features.norm(dim=-1, keepdim=True)
            text_features /= text_features.norm(dim=-1, keepdim=True)
            
            # 计算相似度
            similarity = (image_features @ text_features.T).cpu().numpy()[0][0]
            
        return float(similarity)
```

### 2.3 工作流执行器

```python
# algorithm/app/service/workflow_executor.py
from langgraph.graph import Graph
from app.langgraph import DesignGenerationGraph
import asyncio

class WorkflowExecutor:
    """工作流执行器"""
    
    def __init__(self):
        self.graph = DesignGenerationGraph()
        self.redis_client = Redis()
    
    async def execute(self, task_id: str, params: Dict[str, Any]):
        """执行生成工作流"""
        try:
            # 初始化状态
            initial_state = {
                "task_id": task_id,
                "prompt": params["prompt"],
                "style": params.get("style", "default"),
                "size": params.get("size", (1024, 1024)),
                "reference_images": params.get("reference_images", []),
                "metadata": {
                    "start_time": time.time(),
                    "user_id": params.get("user_id"),
                    "project_id": params.get("project_id")
                }
            }
            
            # 执行工作流
            async for state in self.graph.graph.astream(initial_state):
                # 更新进度
                await self._update_progress(task_id, state)
                
                # 检查是否需要中断
                if await self._should_abort(task_id):
                    raise WorkflowAbortedException("Task aborted by user")
            
            # 保存最终结果
            await self._save_result(task_id, state)
            
        except Exception as e:
            await self._handle_error(task_id, e)
            raise
    
    async def _update_progress(self, task_id: str, state: Dict):
        """更新任务进度"""
        progress = self._calculate_progress(state)
        
        await self.redis_client.hset(
            f"task:progress:{task_id}",
            mapping={
                "phase": progress["phase"],
                "percentage": progress["percentage"],
                "message": progress["message"],
                "updated_at": time.time()
            }
        )
        
        # 发送WebSocket通知
        await self._notify_progress(task_id, progress)
    
    def _calculate_progress(self, state: Dict) -> Dict:
        """计算当前进度"""
        phases = [
            ("parse_intent", 10, "解析设计需求"),
            ("generate_elements", 20, "生成设计元素"),
            ("generate_image", 50, "生成图像"),
            ("ocr_check", 70, "检查文字正确性"),
            ("quality_assessment", 85, "评估图像质量"),
            ("finalize", 100, "完成生成")
        ]
        
        current_phase = state.get("_current_node", "parse_intent")
        
        for phase, percentage, message in phases:
            if phase == current_phase:
                return {
                    "phase": phase,
                    "percentage": percentage,
                    "message": message
                }
        
        return {"phase": "unknown", "percentage": 0, "message": "处理中"}
```

## 3. 模型服务化设计

### 3.1 模型推理服务

```python
# algorithm/models/inference_server.py
from tritonclient.grpc import InferenceServerClient
import numpy as np

class ModelInferenceService:
    """模型推理服务"""
    
    def __init__(self):
        self.triton_client = InferenceServerClient(url='localhost:8001')
        self.models = {
            "sd_xl": "stable_diffusion_xl",
            "controlnet": "controlnet_sdxl",
            "clip": "clip_vit_b32",
            "ocr": "paddle_ocr"
        }
    
    async def generate_image(self, prompt: str, **kwargs):
        """调用Stable Diffusion生成图像"""
        # 准备输入
        inputs = [
            InferInput("prompt", [1], "BYTES"),
            InferInput("negative_prompt", [1], "BYTES"),
            InferInput("num_steps", [1], "INT32"),
            InferInput("guidance_scale", [1], "FP32")
        ]
        
        inputs[0].set_data_from_numpy(np.array([prompt.encode()], dtype=object))
        inputs[1].set_data_from_numpy(
            np.array([kwargs.get("negative_prompt", "").encode()], dtype=object)
        )
        inputs[2].set_data_from_numpy(
            np.array([kwargs.get("num_steps", 30)], dtype=np.int32)
        )
        inputs[3].set_data_from_numpy(
            np.array([kwargs.get("guidance_scale", 7.5)], dtype=np.float32)
        )
        
        # 执行推理
        outputs = await self.triton_client.infer(
            model_name=self.models["sd_xl"],
            inputs=inputs
        )
        
        # 解析结果
        image_array = outputs.as_numpy("generated_image")
        return self._array_to_image(image_array)
    
    async def run_ocr(self, image: np.ndarray):
        """执行OCR识别"""
        inputs = [InferInput("image", image.shape, "UINT8")]
        inputs[0].set_data_from_numpy(image)
        
        outputs = await self.triton_client.infer(
            model_name=self.models["ocr"],
            inputs=inputs
        )
        
        return self._parse_ocr_output(outputs)
```

### 3.2 模型版本管理

```python
# algorithm/models/model_manager.py
import mlflow
from mlflow.tracking import MlflowClient

class ModelManager:
    """模型版本管理"""
    
    def __init__(self):
        self.client = MlflowClient()
        mlflow.set_tracking_uri("http://mlflow-server:5000")
    
    def register_model(self, model_path: str, model_name: str, tags: Dict):
        """注册新模型"""
        with mlflow.start_run():
            # 记录模型
            mlflow.log_artifact(model_path)
            
            # 记录元数据
            mlflow.log_params(tags)
            
            # 注册到模型仓库
            mlflow.register_model(
                f"file://{model_path}",
                model_name
            )
    
    def get_latest_model(self, model_name: str, stage: str = "Production"):
        """获取最新的生产模型"""
        versions = self.client.get_latest_versions(
            name=model_name,
            stages=[stage]
        )
        
        if versions:
            return versions[0]
        return None
    
    def promote_model(self, model_name: str, version: int, stage: str):
        """提升模型版本"""
        self.client.transition_model_version_stage(
            name=model_name,
            version=version,
            stage=stage
        )
```

## 4. 智能优化策略

### 4.1 提示词优化

```python
# algorithm/optimization/prompt_optimizer.py
class PromptOptimizer:
    """提示词优化器"""
    
    def __init__(self):
        self.llm = ChatOpenAI(model="gpt-4")
        self.history_db = PromptHistoryDB()
    
    async def optimize_prompt(self, user_prompt: str, style: str) -> str:
        """优化用户提示词"""
        # 获取相似的成功案例
        similar_cases = await self.history_db.find_similar_successful(
            user_prompt, 
            limit=5
        )
        
        # 构建优化提示
        optimization_prompt = f"""
        用户原始需求：{user_prompt}
        设计风格：{style}
        
        参考成功案例：
        {self._format_cases(similar_cases)}
        
        请优化为Stable Diffusion的提示词，要求：
        1. 保留用户核心需求
        2. 添加适合{style}风格的描述词
        3. 包含质量提升词（如high quality, detailed等）
        4. 避免冲突的描述
        """
        
        optimized = await self.llm.apredict(optimization_prompt)
        
        return optimized
    
    def _format_cases(self, cases):
        """格式化案例"""
        formatted = []
        for case in cases:
            formatted.append(
                f"原始：{case['original']}\n"
                f"优化后：{case['optimized']}\n"
                f"评分：{case['score']}"
            )
        return "\n\n".join(formatted)
```

### 4.2 自适应参数调优

```python
# algorithm/optimization/parameter_tuner.py
class AdaptiveParameterTuner:
    """自适应参数调优器"""
    
    def __init__(self):
        self.param_history = defaultdict(list)
        self.optimizer = BayesianOptimization()
    
    async def get_optimal_params(self, prompt_type: str, style: str) -> Dict:
        """获取最优参数"""
        # 定义参数空间
        param_space = {
            "num_inference_steps": (20, 50),
            "guidance_scale": (5.0, 15.0),
            "controlnet_scale": (0.3, 0.8),
            "temperature": (0.7, 1.0)
        }
        
        # 获取历史数据
        history = self.param_history[f"{prompt_type}_{style}"]
        
        if len(history) < 10:
            # 使用默认参数
            return self._get_default_params(style)
        
        # 贝叶斯优化
        best_params = self.optimizer.optimize(
            objective=lambda p: self._evaluate_params(p, history),
            bounds=param_space,
            n_iter=10
        )
        
        return best_params
    
    def record_result(self, params: Dict, score: float):
        """记录参数效果"""
        key = f"{params['prompt_type']}_{params['style']}"
        self.param_history[key].append({
            "params": params,
            "score": score,
            "timestamp": time.time()
        })
```

## 5. 错误处理与容错

### 5.1 重试机制

```python
# algorithm/utils/retry.py
from tenacity import retry, stop_after_attempt, wait_exponential

class RetryableWorkflow:
    """可重试的工作流"""
    
    @retry(
        stop=stop_after_attempt(3),
        wait=wait_exponential(multiplier=1, min=4, max=10)
    )
    async def generate_with_retry(self, state: DesignState) -> DesignState:
        """带重试的生成"""
        try:
            return await self._generate(state)
        except ModelOverloadError:
            # 模型过载，等待后重试
            await asyncio.sleep(5)
            raise
        except NetworkError:
            # 网络错误，立即重试
            raise
        except Exception as e:
            # 其他错误，记录后失败
            logger.error(f"Generation failed: {e}")
            raise
```

### 5.2 降级策略

```python
# algorithm/utils/fallback.py
class FallbackStrategy:
    """降级策略"""
    
    def __init__(self):
        self.primary_model = "stable_diffusion_xl"
        self.fallback_models = ["stable_diffusion_2", "dall_e_2"]
    
    async def generate_with_fallback(self, prompt: str, **kwargs):
        """带降级的生成"""
        # 尝试主模型
        try:
            return await self._generate_with_model(
                self.primary_model, 
                prompt, 
                **kwargs
            )
        except Exception as e:
            logger.warning(f"Primary model failed: {e}")
        
        # 尝试备用模型
        for model in self.fallback_models:
            try:
                logger.info(f"Falling back to {model}")
                return await self._generate_with_model(
                    model, 
                    prompt, 
                    **kwargs
                )
            except Exception as e:
                logger.warning(f"Fallback model {model} failed: {e}")
                continue
        
        # 所有模型都失败，使用模板
        return await self._use_template_fallback(prompt, **kwargs)
```

## 6. 性能优化

### 6.1 批处理优化

```python
# algorithm/optimization/batch_processor.py
class BatchProcessor:
    """批处理优化器"""
    
    def __init__(self, batch_size: int = 4):
        self.batch_size = batch_size
        self.queue = asyncio.Queue()
        self.processing = False
    
    async def add_task(self, task: Dict) -> str:
        """添加任务到批处理队列"""
        task_id = str(uuid.uuid4())
        await self.queue.put({
            "id": task_id,
            "data": task,
            "future": asyncio.Future()
        })
        
        if not self.processing:
            asyncio.create_task(self._process_batch())
        
        return task_id
    
    async def _process_batch(self):
        """处理批次"""
        self.processing = True
        
        while not self.queue.empty():
            batch = []
            
            # 收集批次
            for _ in range(min(self.batch_size, self.queue.qsize())):
                if not self.queue.empty():
                    batch.append(await self.queue.get())
            
            if batch:
                # 批量处理
                results = await self._batch_generate(
                    [item["data"] for item in batch]
                )
                
                # 分发结果
                for item, result in zip(batch, results):
                    item["future"].set_result(result)
        
        self.processing = False
```

### 6.2 缓存策略

```python
# algorithm/optimization/cache_manager.py
class CacheManager:
    """缓存管理器"""
    
    def __init__(self):
        self.redis_client = Redis()
        self.local_cache = LRUCache(maxsize=100)
    
    async def get_or_generate(self, key: str, generator_func, ttl: int = 3600):
        """获取或生成结果"""
        # 检查本地缓存
        if key in self.local_cache:
            return self.local_cache[key]
        
        # 检查Redis缓存
        cached = await self.redis_client.get(key)
        if cached:
            result = pickle.loads(cached)
            self.local_cache[key] = result
            return result
        
        # 生成新结果
        result = await generator_func()
        
        # 存储到缓存
        await self.redis_client.setex(
            key, 
            ttl, 
            pickle.dumps(result)
        )
        self.local_cache[key] = result
        
        return result
    
    def generate_cache_key(self, params: Dict) -> str:
        """生成缓存键"""
        # 规范化参数
        normalized = {
            k: v for k, v in sorted(params.items())
            if k not in ["task_id", "timestamp"]
        }
        
        # 生成哈希
        param_str = json.dumps(normalized, sort_keys=True)
        return hashlib.md5(param_str.encode()).hexdigest()
```

## 7. 监控与日志

### 7.1 性能监控

```python
# algorithm/monitoring/metrics.py
from prometheus_client import Counter, Histogram, Gauge

# 定义指标
generation_counter = Counter(
    'image_generation_total',
    'Total number of image generations',
    ['model', 'status']
)

generation_duration = Histogram(
    'image_generation_duration_seconds',
    'Duration of image generation',
    ['model', 'phase']
)

active_generations = Gauge(
    'active_generation_tasks',
    'Number of active generation tasks'
)

model_load_time = Histogram(
    'model_load_duration_seconds',
    'Model loading time',
    ['model_name']
)

# 使用示例
class MetricsCollector:
    @staticmethod
    def record_generation(model: str, status: str, duration: float):
        generation_counter.labels(model=model, status=status).inc()
        generation_duration.labels(model=model, phase='total').observe(duration)
    
    @staticmethod
    def track_active_task():
        active_generations.inc()
        try:
            yield
        finally:
            active_generations.dec()
```

### 7.2 日志追踪

```python
# algorithm/monitoring/tracing.py
from opentelemetry import trace
from opentelemetry.trace import Status, StatusCode

tracer = trace.get_tracer(__name__)

class WorkflowTracer:
    """工作流追踪器"""
    
    @staticmethod
    def trace_node(node_name: str):
        """追踪节点执行"""
        def decorator(func):
            async def wrapper(*args, **kwargs):
                with tracer.start_as_current_span(f"node.{node_name}") as span:
                    try:
                        # 记录输入
                        span.set_attribute("node.name", node_name)
                        span.set_attribute("input.size", len(str(args)))
                        
                        # 执行函数
                        result = await func(*args, **kwargs)
                        
                        # 记录成功
                        span.set_status(Status(StatusCode.OK))
                        return result
                        
                    except Exception as e:
                        # 记录错误
                        span.set_status(
                            Status(StatusCode.ERROR, str(e))
                        )
                        span.record_exception(e)
                        raise
            
            return wrapper
        return decorator
```