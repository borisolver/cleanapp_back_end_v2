#!/usr/bin/env bash
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y git curl ffmpeg libsndfile1 build-essential

mkdir -p /opt/cleanapp-ai/voice /opt/cleanapp-ai/voices /opt/models/hf

curl -LsSf https://astral.sh/uv/install.sh | sh
export PATH="/root/.local/bin:$PATH"
uv python install 3.12
uv venv --python 3.12 /opt/cleanapp-ai/voice/.venv
source /opt/cleanapp-ai/voice/.venv/bin/activate
python -m pip install --upgrade pip
python -m pip install qwen-tts fastapi 'uvicorn[standard]' python-multipart soundfile

curl -fsSL https://raw.githubusercontent.com/borisolver/cleanapp_back_end_v2/ai-media-stack/ai-media/voice/server.py \
  -o /opt/cleanapp-ai/voice/server.py

API_KEY="$(curl -fsS -H 'Metadata-Flavor: Google' \
  http://metadata.google.internal/computeMetadata/v1/instance/attributes/ai-api-key)"
cat >/etc/cleanapp-ai-voice.env <<EOF
AI_MEDIA_API_KEY=${API_KEY}
HF_HOME=/opt/models/hf
VOICE_DIR=/opt/cleanapp-ai/voices
QWEN_TTS_MODEL=Qwen/Qwen3-TTS-12Hz-1.7B-Base
EOF
chmod 600 /etc/cleanapp-ai-voice.env

cat >/etc/systemd/system/cleanapp-ai-voice.service <<'EOF'
[Unit]
Description=CleanApp Qwen3-TTS Voice Clone API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/cleanapp-ai-voice.env
WorkingDirectory=/opt/cleanapp-ai/voice
ExecStart=/opt/cleanapp-ai/voice/.venv/bin/uvicorn server:app --host 0.0.0.0 --port 8880 --workers 1
Restart=always
RestartSec=10
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now cleanapp-ai-voice.service
