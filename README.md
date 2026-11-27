# Personal DSM Server (Ubuntu 통합 개인 서버)

`filebrowser`를 대체하고 Synology DSM 스타일 웹 데스크톱 위에서 파일 탐색, 코드 편집, MP4 영상 스트리밍(자막·이어보기), 외부 공유 링크, 그리고 워게임(CTF) 자동화(Phase 2)를 통합 제공하는 개인 서버 프로젝트입니다.

## 현재 구현 범위 (Phase 1 ~ Phase 4 완료)

- **단일 바이너리 구조**: Vue 3(TypeScript) 프론트엔드 빌드 산출물을 Go `embed`로 단일 실행 파일에 통합
- **Public / Private 이중 리스너 보안 분리**:
  - `public_addr` (기본 `127.0.0.1:8080`): Nginx Proxy Manager(DuckDNS) 연결용. 파일·영상·임시 공유 링크·모니터링만 노출
  - `private_addr` (기본 `127.0.0.1:8081`, 실서버에서는 Tailscale IP 바인딩 권장): 마운트 관리·워게임 실행기·웹 터미널 활성화
- **인증 및 보안**: Argon2id 비밀번호 해싱, `HttpOnly` + `SameSite=Strict` 세션 쿠키, IP별 로그인 시도 제한(Rate Limit)
- **Synology DSM 7.2 스타일 웹 데스크톱**: 8방향 창 리사이즈·드래그·최소화·최대화, 36px 상단 메뉴바, 검색형 메인 메뉴 오버레이, 3종 배경화면, 우측 실시간 CPU/RAM/스토리지 위젯, 모바일 반응형 모드
- **동적 마운트 관리 (Phase 1.5)**: 웹 UI에서 서버 로컬 디렉토리 탐색 피커로 실시간 마운트/해제, 읽기 전용(`RO`) 및 Tailscale 내부망 전용 노출 분리
- **Wargame Runner (Phase 2)**:
  - 압축 파일(`.zip`, `.tar.gz`, `.tgz`, `.tar.xz`, `.tar`) 업로드 및 File Station 마운트 폴더/압축 파일 직접 실행 (Zip Slip·심볼릭 링크 차단)
  - `docker-compose.yml` → `Dockerfile` → 단독 파일(`pwn-socat`, `python-flask`, `php-apache`, `node-express`) 3단계 자동 감지기 및 Compose 위험 옵션 제거
  - `ps-wargame-net` 컨테이너 간 통신 차단(`enable_icc=false`), CPU/메모리/PID 및 Capabilities 제한, 지정 바인드 IP 동적 포트 할당
  - 이미지 재사용 1초 리셋, 일시 중지(이미지 보존), 완전 종료(컨테이너·문제 이미지·작업 폴더 일괄 삭제), TTL 자동 정리 및 원클릭 컨테이너 쉘 접속
- **미디어 트랜스코딩 & 서버 관리 (Phase 3)**:
  - `ffprobe` 코덱 분석 기반 Direct Play / 온디맨드 HLS 자동 분기, Intel VAAPI(`/dev/dri/renderD128`) 하드웨어 가속 및 `libx264` 자동 폴백, 60초 유휴 FFmpeg 자동 정리, SHA256 캐시 비디오 썸네일
  - `gopsutil` 2초 단일 수집기 + WebSocket 실시간 리소스 모니터(CPU 전체/코어별, RAM/스왑, 네트워크 RX/TX, 스토리지 볼륨 SVG 차트)
  - `@xterm/xterm` + `creack/pty` 멀티탭 Web Terminal (호스트 로그인 쉘 및 실행 중인 모든 Docker/워게임 컨테이너 `docker exec` 접속, Private 리스너 전용)
- **Docker 관리 앱 & 커스텀 영상 플레이어 (Phase 4)**:
  - Portainer CE를 대체하는 **Docker 관리** 앱 (개요·`docker system df`, 컨테이너 시작/중지/재시작/로그/Inspect/쉘 접속, CodeMirror YAML Compose 스택 편집·배포, 이미지 Pull/Prune, 볼륨·네트워크 관리, Private 리스너 전용)
  - 직접 제작한 커스텀 **Media Player** (`←/→` 5초/10초/30초 이동 및 OSD, `Space/K`/`↑/↓`/`F`/`P`/`[/]`/`C`/`,/.` 단축키, 로컬 `.srt`/`.smi`/`.vtt` 드래그 앤 드롭·EUC-KR 자동 디코딩·실시간 자막 싱크·스타일 설정, 같은 폴더 연속 재생 플레이리스트)
- **File Station**:
  - 지정된 마운트 루트 디렉토리 격리 및 심볼릭 링크 탈출(`EvalSymlinks`) 차단
  - 대용량 파일 청크 업로드(드래그 앤 드롭 지원), 새 폴더/새 파일/이름 변경/삭제
  - 단일 파일 다운로드 및 폴더 실시간 스트리밍 ZIP 다운로드
  - 임시 공유 링크(`/s/{token}`) 발급·폐기 (만료 시간, 비밀번호 보호, 다운로드 허용 여부 설정)
- **Video Player**: HTTP `206 Partial Content` Range 직접 스트리밍, 같은 폴더의 `.srt`/`.smi`(UTF-8 및 EUC-KR 자동 감지) → WebVTT 실시간 자막 변환, SQLite 기반 시청 위치 자동 저장 및 이어보기
- **Code Editor**: CodeMirror 6 구문 강조 에디터, `⌘/Ctrl + S` 단축키 및 임시 파일 `os.Rename` 원자적 저장

## 빌드 및 실행 방법

```bash
# 프론트엔드 빌드 + Go 단일 바이너리 빌드
make build

# 기본 개발 설정으로 즉시 실행 (최초 계정: admin / admin1234!)
./bin/personal-server
```

실행 후 접속 주소:
- Public 리스너 (외부 공개 시뮬레이션): http://127.0.0.1:8080
- Private 리스너 (Tailscale 내부망 시뮬레이션): http://127.0.0.1:8081

## 우분투 서버 배포

1. `deploy/config.example.yaml`을 `/opt/personal-server/config.yaml`로 복사해 마운트 경로와 Tailscale IP(`private_addr`)를 수정합니다.
2. Linux용 바이너리를 크로스 빌드합니다.
   ```bash
   cd web && npm run build && cd ..
   GOOS=linux GOARCH=amd64 go build -o bin/personal-server-linux-amd64 ./cmd/server
   ```
3. `deploy/personal-server.service`를 `/etc/systemd/system/`에 등록하고 `systemctl enable --now personal-server`로 실행합니다.
4. Nginx Proxy Manager에서는 DuckDNS 도메인을 `public_addr`(`127.0.0.1:8080`)으로만 포워딩합니다.
