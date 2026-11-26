# Personal DSM Server (Ubuntu 통합 개인 서버)

`filebrowser`를 대체하고 Synology DSM 스타일 웹 데스크톱 위에서 파일 탐색, 코드 편집, MP4 영상 스트리밍(자막·이어보기), 외부 공유 링크, 그리고 워게임(CTF) 자동화(Phase 2)를 통합 제공하는 개인 서버 프로젝트입니다.

## 현재 구현 범위 (Phase 1 완료)

- **단일 바이너리 구조**: Vue 3(TypeScript) 프론트엔드 빌드 산출물을 Go `embed`로 단일 실행 파일에 통합
- **Public / Private 이중 리스너 보안 분리**:
  - `public_addr` (기본 `127.0.0.1:8080`): Nginx Proxy Manager(DuckDNS) 연결용. 파일·영상·임시 공유 링크만 노출
  - `private_addr` (기본 `127.0.0.1:8081`, 실서버에서는 Tailscale IP 바인딩 권장): 모든 관리 기능 및 Tailscale 전용 앱 활성화
- **인증 및 보안**: Argon2id 비밀번호 해싱, `HttpOnly` + `SameSite=Strict` 세션 쿠키, IP별 로그인 시도 제한(Rate Limit)
- **DSM 웹 데스크톱**: 창 드래그·리사이즈·최소화·최대화·포커스 관리, 상단 작업표시줄, 우측 디스크/시스템 상태 위젯, 모바일(768px 미만) 전체 화면 반응형 모드
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
