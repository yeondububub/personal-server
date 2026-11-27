#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/personal-server}"
SERVICE_USER="${SERVICE_USER:-$USER}"
SERVICE_GROUP="${SERVICE_GROUP:-$(id -gn "$SERVICE_USER")}"
BIN_SOURCE="${1:-./bin/personal-server-linux-amd64}"

if [[ ! -f "$BIN_SOURCE" ]]; then
  echo "오류: 바이너리 파일을 찾을 수 없습니다 ($BIN_SOURCE)" >&2
  echo "사용법: ./deploy/install.sh [바이너리_경로]" >&2
  exit 1
fi

echo "[1/5] 필수 패키지(ffmpeg) 확인..."
if ! command -v ffmpeg >/dev/null 2>&1; then
  sudo apt-get update
  sudo apt-get install -y ffmpeg
fi

echo "[2/5] 사용자 그룹(docker, render, video) 권한 설정..."
for grp in docker render video; do
  if getent group "$grp" >/dev/null 2>&1; then
    sudo usermod -aG "$grp" "$SERVICE_USER" || true
  fi
done

echo "[3/5] 바이너리 및 설정 설치 ($INSTALL_DIR)..."
sudo mkdir -p "$INSTALL_DIR"
sudo chown -R "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR"
install -m 0755 "$BIN_SOURCE" "$INSTALL_DIR/personal-server.new"
mv "$INSTALL_DIR/personal-server.new" "$INSTALL_DIR/personal-server"

if [[ ! -f "$INSTALL_DIR/config.yaml" ]]; then
  TAILSCALE_IP=""
  if command -v tailscale >/dev/null 2>&1; then
    TAILSCALE_IP="$(tailscale ip -4 2>/dev/null || true)"
  fi
  PRIVATE_ADDR="127.0.0.1:8081"
  WG_BIND_IP="127.0.0.1"
  if [[ -n "$TAILSCALE_IP" ]]; then
    PRIVATE_ADDR="${TAILSCALE_IP}:8081"
    WG_BIND_IP="${TAILSCALE_IP}"
  fi

  cat <<EOF > "$INSTALL_DIR/config.yaml"
server:
  public_addr: "0.0.0.0:8080"
  private_addr: "${PRIVATE_ADDR}"
  public_base_url: ""
  cookie_secure: false
  allow_public_admin: false

storage:
  database_path: "./data/personal-server.db"
  temp_upload_dir: "./data/uploads"
  stacks_dir: "./data/stacks"
  roots:
    - id: "home"
      name: "Home (${SERVICE_USER})"
      path: "$(eval echo "~${SERVICE_USER}")"

auth:
  initial_admin_username: "admin"
  initial_admin_password: "changeme123!"
  session_ttl_hours: 168

wargame:
  bind_ip: "${WG_BIND_IP}"
  port_start: 30000
  port_end: 39999
  work_dir: "./data/wargames"
  default_ttl_hours: 3
  max_archive_bytes: 1073741824
EOF
  echo "  -> 기본 config.yaml 생성 완료 (최초 계정: admin / changeme123!)"
fi

echo "[4/5] systemd 서비스 등록..."
sudo tee /etc/systemd/system/personal-server.service >/dev/null <<EOF
[Unit]
Description=Personal Ubuntu Server Integrated Environment
After=network-online.target docker.service tailscaled.service
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/personal-server -config ${INSTALL_DIR}/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

echo "[5/5] 서비스 시작..."
sudo systemctl daemon-reload
sudo systemctl enable --now personal-server
sudo systemctl restart personal-server
sudo systemctl status personal-server --no-pager
