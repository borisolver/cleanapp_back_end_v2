import gc
import os
import tempfile
import threading
import uuid
from pathlib import Path
from typing import Optional

import torch
from diffusers import DiffusionPipeline
from diffusers.utils import export_to_video, load_image
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile
from fastapi.responses import FileResponse
from pydantic import BaseModel

API_KEY = os.environ["AI_MEDIA_API_KEY"]
MODEL_T2V = os.getenv("WAN_T2V_MODEL", "Wan-AI/Wan2.2-T2V-A14B-Diffusers")
MODEL_I2V = os.getenv("WAN_I2V_MODEL", "Wan-AI/Wan2.2-I2V-A14B-Diffusers")
OUT_DIR = Path(os.getenv("VIDEO_OUT_DIR", "/opt/cleanapp-ai/video-output"))
OUT_DIR.mkdir(parents=True, exist_ok=True)

app = FastAPI(title="CleanApp Video API", version="1.0.0")
_lock = threading.Lock()
_pipe = None
_pipe_kind = None


def auth(x_api_key: Optional[str]):
    if x_api_key != API_KEY:
        raise HTTPException(status_code=401, detail="invalid api key")


def clear_pipe():
    global _pipe, _pipe_kind
    if _pipe is not None:
        del _pipe
    _pipe = None
    _pipe_kind = None
    gc.collect()
    if torch.cuda.is_available():
        torch.cuda.empty_cache()


def get_pipe(kind: str):
    global _pipe, _pipe_kind
    model_id = MODEL_T2V if kind == "t2v" else MODEL_I2V
    if _pipe is None or _pipe_kind != kind:
        clear_pipe()
        os.environ.setdefault("HF_ENABLE_PARALLEL_LOADING", "YES")
        _pipe = DiffusionPipeline.from_pretrained(
            model_id,
            dtype=torch.bfloat16,
            device_map="cuda",
        )
        _pipe_kind = kind
    return _pipe


@app.get("/health")
def health():
    return {
        "ok": True,
        "cuda": torch.cuda.is_available(),
        "gpu": torch.cuda.get_device_name(0) if torch.cuda.is_available() else None,
        "loaded_pipeline": _pipe_kind,
        "t2v_model": MODEL_T2V,
        "i2v_model": MODEL_I2V,
    }


class T2VRequest(BaseModel):
    prompt: str
    negative_prompt: Optional[str] = None
    width: int = 1280
    height: int = 720
    num_frames: int = 81
    steps: int = 40
    guidance_scale: float = 5.0
    fps: int = 16
    seed: Optional[int] = None


@app.post("/v1/video/text-to-video")
def text_to_video(
    req: T2VRequest,
    x_api_key: Optional[str] = Header(None, alias="X-API-Key"),
):
    auth(x_api_key)
    with _lock:
        pipe = get_pipe("t2v")
        generator = None
        if req.seed is not None:
            generator = torch.Generator(device="cuda").manual_seed(req.seed)
        result = pipe(
            prompt=req.prompt,
            negative_prompt=req.negative_prompt,
            width=req.width,
            height=req.height,
            num_frames=req.num_frames,
            num_inference_steps=req.steps,
            guidance_scale=req.guidance_scale,
            generator=generator,
        )
        frames = result.frames[0]
        out = OUT_DIR / f"t2v-{uuid.uuid4().hex}.mp4"
        export_to_video(frames, str(out), fps=req.fps)
        return FileResponse(out, media_type="video/mp4", filename=out.name)


@app.post("/v1/video/image-to-video")
async def image_to_video(
    prompt: str = Form(...),
    image: UploadFile = File(...),
    negative_prompt: Optional[str] = Form(None),
    width: int = Form(1280),
    height: int = Form(720),
    num_frames: int = Form(81),
    steps: int = Form(40),
    guidance_scale: float = Form(5.0),
    fps: int = Form(16),
    seed: Optional[int] = Form(None),
    x_api_key: Optional[str] = Header(None, alias="X-API-Key"),
):
    auth(x_api_key)
    suffix = Path(image.filename or ".png").suffix
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as f:
        f.write(await image.read())
        image_path = f.name
    try:
        with _lock:
            pipe = get_pipe("i2v")
            generator = None
            if seed is not None:
                generator = torch.Generator(device="cuda").manual_seed(seed)
            src = load_image(image_path)
            result = pipe(
                image=src,
                prompt=prompt,
                negative_prompt=negative_prompt,
                width=width,
                height=height,
                num_frames=num_frames,
                num_inference_steps=steps,
                guidance_scale=guidance_scale,
                generator=generator,
            )
            frames = result.frames[0]
            out = OUT_DIR / f"i2v-{uuid.uuid4().hex}.mp4"
            export_to_video(frames, str(out), fps=fps)
            return FileResponse(out, media_type="video/mp4", filename=out.name)
    finally:
        try:
            os.unlink(image_path)
        except OSError:
            pass
