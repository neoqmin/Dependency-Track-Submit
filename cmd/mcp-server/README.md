# dtrack-mcp-server

Dependency-Track REST API를 MCP(Model Context Protocol)로 노출하는 stdio 서버입니다.
kernforge, VS Code(GitHub Copilot), Claude Desktop 등 MCP를 지원하는 모든 도구에서
자연어로 취약점 조회·분석·수정을 수행할 수 있습니다.

> **별도 실행 불필요.** MCP 호스트(kernforge, VS Code 등)가 설정 파일을 읽어
> 서버 프로세스를 자동으로 시작/종료합니다.

---

## 빌드

```powershell
cd D:\Sources\Tools\DependencyTrack\dtrack-submit
go build -o cmd\mcp-server\dtrack-mcp-server.exe .\cmd\mcp-server\
```

---

## 설정

API 키와 서버 주소는 **MCP 호스트 설정 파일의 `env` 블록**에 직접 지정합니다.
서버가 subprocess로 실행될 때 해당 값이 환경변수로 자동 주입됩니다.

| 환경변수 | 설명 | 기본값 |
|----------|------|--------|
| `DTRACK_URL` | Dependency-Track 서버 주소 | `http://localhost:8080` |
| `DTRACK_API_KEY` | API 키 | (없으면 경고, 도구 호출 실패) |

> `dtrack.json` 파일(`{"server":"...","api_key":"..."}`)을 실행 디렉토리나
> `~/.dtrack.json`에 두는 것도 가능하지만, 환경변수가 우선 적용됩니다.

### API 키 발급

1. Dependency-Track 웹 UI 접속 (`http://localhost:8080`)
2. **Administration → Access Management → Teams → Administrators**
3. **API Keys** 탭 → **Generate API Key** 복사

---

## kernforge 등록

`~/.kernforge/config.json`의 `mcp_servers` 배열에 추가합니다.

```json
{
  "mcp_servers": [
    {
      "name": "dependency-track",
      "command": "D:\\Sources\\Tools\\DependencyTrack\\dtrack-submit\\cmd\\mcp-server\\dtrack-mcp-server.exe",
      "args": [],
      "env": {
        "DTRACK_URL": "http://localhost:8080",
        "DTRACK_API_KEY": "odt_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
      },
      "cwd": "."
    }
  ]
}
```

### 등록 확인

kernforge 실행 후:

```
/status
```

출력에서 `dependency-track` 서버와 7개 도구(`dtrack_list_projects` 등)가 보이면 성공입니다.

---

## VS Code 등록

`%APPDATA%\Code\User\mcp.json`의 `servers`에 추가합니다.

```json
{
  "servers": {
    "dependency-track": {
      "type": "stdio",
      "command": "D:\\Sources\\Tools\\DependencyTrack\\dtrack-submit\\cmd\\mcp-server\\dtrack-mcp-server.exe",
      "args": [],
      "env": {
        "DTRACK_URL": "http://localhost:8080",
        "DTRACK_API_KEY": "odt_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### 등록 확인

VS Code에서 **명령 팔레트**(Ctrl+Shift+P) → `MCP: List Servers` 실행 후
`dependency-track` 서버가 보이면 성공입니다.

---

## 제공 도구 (7개)

| 도구 이름 | 설명 |
|-----------|------|
| `dtrack_list_projects` | 등록된 프로젝트 목록 조회 |
| `dtrack_get_findings` | 프로젝트 취약점 목록 조회 (severity 필터 가능) |
| `dtrack_get_project_metrics` | 취약점 통계 및 위험 점수 조회 |
| `dtrack_get_component_remediation` | 특정 컴포넌트의 CVE 및 수정 버전 조회 |
| `dtrack_upload_bom` | SBOM 생성 후 업로드 (syft 또는 lockfile 파싱) |
| `dtrack_submit_project` | 프로젝트 타입 자동 감지 후 등록 및 분석 요청 |
| `dtrack_generate_report` | 취약점 리포트 생성 (패키지별 그룹화, 최신 버전 포함) |

---

## 사용 예시

### kernforge

```
you > 현재 프로젝트 취약점 분석해줘
  → dtrack_submit_project 호출 (SBOM 생성 + 업로드)
  → dtrack_generate_report 호출 (리포트 출력)

you > CRITICAL 취약점만 보여줘
  → dtrack_get_findings(severity="CRITICAL") 호출

you > lodash 취약점 수정해줘
  → dtrack_get_component_remediation(component_name="lodash") 호출
  → package.json 직접 수정
```

### VS Code (GitHub Copilot Chat)

```
@dependency-track 등록된 프로젝트 목록 보여줘
@dependency-track myapp HIGH 이상 취약점 조회해줘
```

---

## SBOM 생성 우선순위

`dtrack_submit_project`와 `dtrack_upload_bom`은 다음 순서로 SBOM을 생성합니다.

1. 프로젝트 타입 감지 후 전용 생성기 실행
   - Go → `cyclonedx-gomod`
   - Maven → `mvn cyclonedx:makeAggregateBom`
   - Gradle → `gradle cyclonedxBom`
   - .NET → `dotnet CycloneDX`
   - NPM → `@cyclonedx/cyclonedx-npm`
   - 기타 → `cdxgen`
2. 전용 생성기 실패 시 → `syft` (설치된 경우)
3. syft 없을 경우 → lockfile 직접 파싱 (`go.mod` / `package-lock.json` / `requirements.txt`)

---

## 트러블슈팅

### `context deadline exceeded` 경고

서버 바이너리 경로가 잘못되었거나 빌드가 안 된 경우입니다.
경로를 확인하고 빌드를 다시 실행하세요.

```powershell
ls D:\Sources\Tools\DependencyTrack\dtrack-submit\cmd\mcp-server\dtrack-mcp-server.exe
```

### `no API key configured` 경고

`DTRACK_API_KEY` 환경변수 또는 설정 파일의 `api_key`가 비어 있습니다.

### 도구 호출 시 연결 실패

Dependency-Track 서버가 실행 중인지 확인합니다.

```powershell
curl http://localhost:8080/api/version
```
