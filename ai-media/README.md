# CleanApp self-hosted AI media stack

This branch contains a private, self-hosted voice-cloning and video-generation stack intended for Google Cloud GPU VMs.

## Models

- **Voice:** `Qwen/Qwen3-TTS-12Hz-1.7B-Base` — Apache-2.0, multilingual, rapid reference-audio voice cloning.
- **Video:** `Wan-AI/Wan2.2-T2V-A14B-Diffusers` and `Wan-AI/Wan2.2-I2V-A14B-Diffusers` — Apache-2.0 text-to-video and image-to-video.

The model weights are downloaded directly from Hugging Face on first bootstrap and cached on the VM boot disks. There is no per-request vendor API dependency after deployment.

## Provision on GCP

Run from any machine with an authenticated `gcloud` CLI and permissions to create Compute Engine instances:

```bash
chmod +x ai-media/deploy-gcp.sh
./ai-media/deploy-gcp.sh
```

Defaults:

- voice VM: `g2-standard-4` (NVIDIA L4), 250 GB disk
- video VM: `a2-ultragpu-1g` (NVIDIA A100 80 GB), 750 GB disk
- network: `default`
- voice port: `8880`
- video port: `8787`

Machine types, zones, disks and names can be overridden with environment variables such as `VIDEO_ZONE`, `VIDEO_MACHINE_TYPE`, `VOICE_ZONE`, and `VOICE_MACHINE_TYPE`.

The deploy script intentionally does **not** create public firewall rules. The services are designed for trusted same-VPC callers and require `X-API-Key` authentication.

## Voice API

Health:

```bash
curl http://VOICE_INTERNAL_IP:8880/health
```

Create a private voice clone from a reference recording:

```bash
curl -X POST "http://VOICE_INTERNAL_IP:8880/v1/voices/boris" \
  -H "X-API-Key: $AI_MEDIA_API_KEY" \
  -F "audio=@reference.m4a" \
  -F 'ref_text=Exact transcript of the reference clip' \
  -F 'language=English'
```

Generate speech:

```bash
curl -X POST "http://VOICE_INTERNAL_IP:8880/v1/speech" \
  -H "X-API-Key: $AI_MEDIA_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"voice_id":"boris","text":"This is a test of the self-hosted voice clone.","language":"English"}' \
  --output boris-test.wav
```

## Video API

Text to video:

```bash
curl -X POST "http://VIDEO_INTERNAL_IP:8787/v1/video/text-to-video" \
  -H "X-API-Key: $AI_MEDIA_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"A cinematic tracking shot through a clean modern European city after rain","width":1280,"height":720,"num_frames":81,"steps":40,"fps":16}' \
  --output test.mp4
```

Image to video:

```bash
curl -X POST "http://VIDEO_INTERNAL_IP:8787/v1/video/image-to-video" \
  -H "X-API-Key: $AI_MEDIA_API_KEY" \
  -F 'prompt=Slow cinematic camera push-in, realistic motion and lighting' \
  -F 'image=@input.png' \
  --output test.mp4
```

## Files

- `voice/server.py` — Qwen3-TTS voice clone/TTS FastAPI service
- `video/server.py` — Wan 2.2 T2V/I2V FastAPI service
- `startup/voice.sh` — voice VM bootstrap
- `startup/video.sh` — video VM bootstrap
- `deploy-gcp.sh` — GCP provisioning and health-check script

Do not commit reference voice recordings, generated API keys, or other biometric/private media into the public repository.
