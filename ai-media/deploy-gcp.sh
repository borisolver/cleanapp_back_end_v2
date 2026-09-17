#!/usr/bin/env bash
set -euo pipefail

# One-command provisioner for the CleanApp self-hosted AI media stack.
# Requires a logged-in gcloud CLI with permission to create Compute Engine VMs.
# It creates two GPU VMs in the current GCP project:
#   - cleanapp-ai-voice: Qwen3-TTS 1.7B voice cloning API
#   - cleanapp-ai-video: Wan 2.2 A14B text/image-to-video API
#
# Override any defaults by exporting variables before running, e.g.:
#   VIDEO_ZONE=us-central1-a VIDEO_MACHINE_TYPE=a2-ultragpu-1g ./deploy-gcp.sh

PROJECT_ID="${PROJECT_ID:-$(gcloud config get-value project 2>/dev/null)}"
if [[ -z "${PROJECT_ID}" || "${PROJECT_ID}" == "(unset)" ]]; then
  echo "No GCP project selected. Run: gcloud config set project <PROJECT_ID>" >&2
  exit 1
fi

VOICE_VM="${VOICE_VM:-cleanapp-ai-voice}"
VIDEO_VM="${VIDEO_VM:-cleanapp-ai-video}"
VOICE_MACHINE_TYPE="${VOICE_MACHINE_TYPE:-g2-standard-4}"      # 1x NVIDIA L4 24 GB
VIDEO_MACHINE_TYPE="${VIDEO_MACHINE_TYPE:-a2-ultragpu-1g}"    # 1x NVIDIA A100 80 GB
VOICE_DISK_GB="${VOICE_DISK_GB:-250}"
VIDEO_DISK_GB="${VIDEO_DISK_GB:-750}"
NETWORK="${NETWORK:-default}"
BRANCH="${BRANCH:-ai-media-stack}"
REPO_RAW="https://raw.githubusercontent.com/borisolver/cleanapp_back_end_v2/${BRANCH}/ai-media/startup"

choose_zone() {
  local machine="$1"
  local preferred="${2:-}"
  if [[ -n "$preferred" ]]; then
    echo "$preferred"
    return
  fi
  gcloud compute machine-types list \
    --project "$PROJECT_ID" \
    --filter="name=${machine}" \
    --format='value(zone.basename())' 2>/dev/null | head -n1
}

VOICE_ZONE="$(choose_zone "$VOICE_MACHINE_TYPE" "${VOICE_ZONE:-}")"
VIDEO_ZONE="$(choose_zone "$VIDEO_MACHINE_TYPE" "${VIDEO_ZONE:-}")"
if [[ -z "$VOICE_ZONE" ]]; then
  echo "Could not find a zone exposing ${VOICE_MACHINE_TYPE}. Set VOICE_ZONE manually." >&2
  exit 1
fi
if [[ -z "$VIDEO_ZONE" ]]; then
  echo "Could not find a zone exposing ${VIDEO_MACHINE_TYPE}. Set VIDEO_ZONE manually." >&2
  exit 1
fi

# Use one shared key for both internal APIs. It is placed only in instance metadata,
# not committed to GitHub.
AI_MEDIA_API_KEY="${AI_MEDIA_API_KEY:-$(openssl rand -hex 32)}"

# Prefer a current NVIDIA Deep Learning VM image. This keeps GPU driver/CUDA setup
# out of the bootstrap scripts.
DL_IMAGE="${DL_IMAGE:-$(gcloud compute images list \
  --project=deeplearning-platform-release \
  --filter="name~'common-cu12.*ubuntu-2204.*nvidia'" \
  --sort-by=~creationTimestamp --limit=1 --format='value(name)' 2>/dev/null)}"
if [[ -z "$DL_IMAGE" ]]; then
  echo "Could not resolve a current NVIDIA Deep Learning VM image." >&2
  exit 1
fi

create_vm() {
  local name="$1" zone="$2" machine="$3" disk="$4" startup_url="$5"
  if gcloud compute instances describe "$name" --zone "$zone" --project "$PROJECT_ID" >/dev/null 2>&1; then
    echo "${name} already exists in ${zone}; leaving it intact."
    return
  fi

  echo "Creating ${name} (${machine}) in ${zone}..."
  gcloud compute instances create "$name" \
    --project "$PROJECT_ID" \
    --zone "$zone" \
    --machine-type "$machine" \
    --network "$NETWORK" \
    --image "$DL_IMAGE" \
    --image-project deeplearning-platform-release \
    --boot-disk-size "${disk}GB" \
    --boot-disk-type pd-balanced \
    --maintenance-policy TERMINATE \
    --restart-on-failure \
    --metadata "ai-api-key=${AI_MEDIA_API_KEY},startup-script-url=${startup_url}"
}

create_vm "$VOICE_VM" "$VOICE_ZONE" "$VOICE_MACHINE_TYPE" "$VOICE_DISK_GB" "${REPO_RAW}/voice.sh"
create_vm "$VIDEO_VM" "$VIDEO_ZONE" "$VIDEO_MACHINE_TYPE" "$VIDEO_DISK_GB" "${REPO_RAW}/video.sh"

printf '\nWaiting for bootstrap services to become healthy. Model downloads can take several minutes.\n'

wait_service() {
  local name="$1" zone="$2" port="$3" service="$4"
  for i in $(seq 1 60); do
    if gcloud compute ssh "$name" --project "$PROJECT_ID" --zone "$zone" \
      --command "curl -fsS http://127.0.0.1:${port}/health" 2>/dev/null; then
      echo
      echo "${name}: healthy"
      return 0
    fi
    if (( i % 6 == 0 )); then
      echo "${name}: still bootstrapping..."
      gcloud compute ssh "$name" --project "$PROJECT_ID" --zone "$zone" \
        --command "sudo systemctl --no-pager --full status ${service} || true" 2>/dev/null || true
    fi
    sleep 20
  done
  echo "${name}: health check did not pass in time." >&2
  return 1
}

voice_ok=0
video_ok=0
wait_service "$VOICE_VM" "$VOICE_ZONE" 8880 cleanapp-ai-voice.service && voice_ok=1 || true
wait_service "$VIDEO_VM" "$VIDEO_ZONE" 8787 cleanapp-ai-video.service && video_ok=1 || true

VOICE_IP="$(gcloud compute instances describe "$VOICE_VM" --project "$PROJECT_ID" --zone "$VOICE_ZONE" --format='value(networkInterfaces[0].networkIP)')"
VIDEO_IP="$(gcloud compute instances describe "$VIDEO_VM" --project "$PROJECT_ID" --zone "$VIDEO_ZONE" --format='value(networkInterfaces[0].networkIP)')"

cat <<EOF

CleanApp AI media stack provisioned.
Project:   ${PROJECT_ID}
Voice VM:  ${VOICE_VM} (${VOICE_ZONE})  internal http://${VOICE_IP}:8880
Video VM:  ${VIDEO_VM} (${VIDEO_ZONE})  internal http://${VIDEO_IP}:8787
Voice OK:  ${voice_ok}
Video OK:  ${video_ok}

API key (store this in Secret Manager / backend config):
${AI_MEDIA_API_KEY}

The APIs are not opened to the public internet by this script. They are intended
for calls from trusted workloads on the same VPC; both additionally require the
X-API-Key header.
EOF
