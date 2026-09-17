import json
import os
import re
import tempfile
from pathlib import Path
from typing import Optional

import soundfile as sf
import torch
from fastapi import FastAPI, File, Form, Header, HTTPException, UploadFile
from fastapi.responses import FileResponse
from pydantic import BaseModel
from qwen_tts import Qwen3TTSModel

API_KEY = os.environ["AI_MEDIA_API_KEY"]
MODEL_ID = os.getenv("QWEN_TTS_MODEL", "Qwen/Qwen3-TTS-12Hz-1.7B-Base")
VOICE_DIR = Path(os.getenv("VOICE_DIR", "/opt/cleanapp-ai/voices"))
VOICE_DIR.mkdir(parents=True, exist_ok=True)

app = FastAPI(title="CleanApp Voice API", version="1.0.0")
model: Optional[Qwen3TTSModel] = None
voice_cache = {}


def auth(x_api_key: Optional[str]):
    if x_api_key != API_KEY:
        raise HTTPException(status_code=401, detail="invalid api key")


def safe_id(value: str) -> str:
    value = re.sub(r"[^A-Za-z0-9_.-]+", "-", value.strip())
    if not value:
        raise HTTPException(status_code=400, detail="invalid voice id")
    return value[:80]


def load_model():
    global model
    if model is None:
        model = Qwen3TTSModel.from_pretrained(
            MODEL_ID,
            device_map="cuda:0",
            dtype=torch.bfloat16,
            attn_implementation="sdpa",
        )
    return model


@app.on_event("startup")
def startup():
    load_model()


@app.get("/health")
def health():
    return {
        "ok": True,
        "model": MODEL_ID,
        "cuda": torch.cuda.is_available(),
        "gpu": torch.cuda.get_device_name(0) if torch.cuda.is_available() else None,
    }


@app.post("/v1/voices/{voice_id}")
async def create_voice(
    voice_id: str,
    audio: UploadFile = File(...),
    ref_text: str = Form(...),
    language: str = Form("English"),
    x_api_key: Optional[str] = Header(None, alias="X-API-Key"),
):
    auth(x_api_key)
    voice_id = safe_id(voice_id)
    target = VOICE_DIR / voice_id
    target.mkdir(parents=True, exist_ok=True)
    audio_path = target / "reference.wav"

    raw = await audio.read()
    with tempfile.NamedTemporaryFile(suffix=Path(audio.filename or ".wav").suffix, delete=False) as f:
        f.write(raw)
        tmp = f.name

    # soundfile handles common WAV/FLAC; ffmpeg conversion is used as fallback.
    try:
        data, sr = sf.read(tmp)
        sf.write(audio_path, data, sr)
    except Exception:
        import subprocess
        subprocess.run([
            "ffmpeg", "-y", "-i", tmp, "-ac", "1", "-ar", "24000", str(audio_path)
        ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    finally:
        try:
            os.unlink(tmp)
        except OSError:
            pass

    meta = {"voice_id": voice_id, "ref_text": ref_text, "language": language}
    (target / "metadata.json").write_text(json.dumps(meta, ensure_ascii=False, indent=2))

    qwen = load_model()
    prompt = qwen.create_voice_clone_prompt(
        ref_audio=str(audio_path),
        ref_text=ref_text,
        x_vector_only_mode=False,
    )
    voice_cache[voice_id] = prompt
    return {"ok": True, **meta}


class SpeechRequest(BaseModel):
    voice_id: str
    text: str
    language: str = "English"


@app.post("/v1/speech")
def speech(
    req: SpeechRequest,
    x_api_key: Optional[str] = Header(None, alias="X-API-Key"),
):
    auth(x_api_key)
    voice_id = safe_id(req.voice_id)
    target = VOICE_DIR / voice_id
    meta_path = target / "metadata.json"
    audio_path = target / "reference.wav"
    if not meta_path.exists() or not audio_path.exists():
        raise HTTPException(status_code=404, detail="voice not found")

    qwen = load_model()
    prompt = voice_cache.get(voice_id)
    if prompt is None:
        meta = json.loads(meta_path.read_text())
        prompt = qwen.create_voice_clone_prompt(
            ref_audio=str(audio_path),
            ref_text=meta["ref_text"],
            x_vector_only_mode=False,
        )
        voice_cache[voice_id] = prompt

    wavs, sr = qwen.generate_voice_clone(
        text=req.text,
        language=req.language,
        voice_clone_prompt=prompt,
    )
    out = target / "latest.wav"
    sf.write(out, wavs[0], sr)
    return FileResponse(out, media_type="audio/wav", filename=f"{voice_id}.wav")


@app.get("/v1/voices")
def list_voices(x_api_key: Optional[str] = Header(None, alias="X-API-Key")):
    auth(x_api_key)
    result = []
    for p in VOICE_DIR.iterdir():
        meta = p / "metadata.json"
        if p.is_dir() and meta.exists():
            result.append(json.loads(meta.read_text()))
    return {"voices": result}
