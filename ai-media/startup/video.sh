#!/usr/bin/env bash
set -euxo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y git curl ffmpeg build-essential

mkdir -p /opt/cleanapp-ai/video /opt/cleanapp-ai/video-output /opt/models/hf

curl -LsSf https://astral.sh/uv/install.sh | sh
export PATH="/root/.local/bin:$PATH"
uv python install 3.12
uv venv --python 3.12 /opt/cleanapp-ai/video/.venv
source /opt/cleanapp-ai/video/.venv/bin/activate
python -m pip install --upgrade pip
python -m pip install --index-url https://download.pytorch.org/whl/cu129 torch torchvision
python -m pip install 'diffusers>=0.40.0' 'transformers>=4.57.0' accelerate safetensors sentencepiece ftfy \
  fastapi 'uvicorn[standard]' python-multipart 'imageio[ffmpeg]' pillow huggingface_hub hf_xet

curl -fsSL https://raw.githubusercontent.com/borisolver/cleanapp_back_end_v2/ai-media-stack/ai-media/video/server.py \
  -o /opt/cleanapp-ai/video/server.py

API_KEY="$(curl -fsS -H 'Metadata-Flavor: Google' \
  http://metadata.google.internal/computeMetadata/v1/instance/attributes/ai-api-key)"
cat >/etc/cleanapp-ai-video.env <<EOF
AI_MEDIA_API_KEY=${API_KEY}
HF_HOME=/opt/models/hf
HF_XET_HIGH_PERFORMANCE=1
HF_ENABLE_PARALLEL_LOADING=YES
VIDEO_OUT_DIR=/opt/cleanapp-ai/video-output
WAN_T2V_MODEL=Wan-AI/Wan2.2-T2V-A14B-Diffusers
WAN_I2V_MODEL=Wan-AI/Wan2.2-I2V-A14B-Diffusers
EOF
chmod 600 /etc/cleanapp-ai-video.env

cat >/etc/systemd/system/cleanapp-ai-video.service <<'EOF'
[Unit]
Description=CleanApp Wan2.2 Video Generation API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/cleanapp-ai-video.env
WorkingDirectory=/opt/cleanapp-ai/video
ExecStart=/opt/cleanapp-ai/video/.venv/bin/uvicorn server:app --host 0.0.0.0 --port 8787 --workers 1
Restart=always
RestartSec=10
TimeoutStartSec=0
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now cleanapp-ai-video.service
