# dtrack-submit

Dependency-Track에 프로젝트 SBOM을 자동으로 등록하는 멀티플랫폼 CLI 툴.

프로젝트 디렉토리를 지정하면 언어/빌드 시스템을 자동으로 감지하여 CycloneDX SBOM을 생성하고 Dependency-Track 서버에 업로드합니다. 모노레포도 지원합니다.

## 지원 플랫폼

| OS | 아키텍처 | 바이너리 |
|----|----------|---------|
| Windows | x64 | `dtrack-submit-windows-amd64.exe` |
| Linux | x64 | `dtrack-submit-linux-amd64` |
| macOS | Intel | `dtrack-submit-darwin-amd64` |
| macOS | Apple Silicon | `dtrack-submit-darwin-arm64` |

## 지원 프로젝트 타입

디렉토리 내 파일을 스캔하여 프로젝트 타입을 자동 감지합니다.

| 감지 파일 | 프로젝트 타입 | SBOM 생성 도구 |
|-----------|--------------|---------------|
| `pom.xml` | Java / Maven | `mvn org.cyclonedx:cyclonedx-maven-plugin` |
| `build.gradle`, `build.gradle.kts` | Java / Gradle | `gradlew cyclonedxBom` |
| `go.mod` | Go | `cyclonedx-gomod` |
| `*.csproj`, `*.sln` | C# / .NET | `dotnet CycloneDX` |
| `CMakeLists.txt`, `conanfile.*`, `vcpkg.json` | C++ | `cdxgen` |
| `package.json` | Node.js / npm | `@cyclonedx/cyclonedx-npm` |
| `Podfile`, `Podfile.lock` | iOS / macOS (CocoaPods) | Podfile.lock 직접 파싱 |
| `Package.swift` | Swift Package Manager | `cdxgen` |
| `Cartfile`, `Cartfile.resolved` | Swift / Carthage | `cdxgen` |
| `*.xcodeproj`, `*.xcworkspace` | Xcode 프로젝트 | `cdxgen` |
| `*.swift` (루트에 존재) | Swift 소스 | `cdxgen` |

언어별 전용 도구가 설치되지 않은 경우 **cdxgen으로 자동 폴백**합니다.

### 모노레포

루트에 manifest가 없으면 하위 디렉토리를 자동 스캔하여 발견된 모든 프로젝트를 각각 등록합니다.

```
→ Scanning D:\Sources\pgTelemetry
  Found 3 sub-projects

  [go]  pgTelemetry/api @ 1.26.2
  [npm] pgTelemetry/dashboard @ 0.1.0
  [cpp] pgTelemetry/sdk-cpp @ 0.0.0
```

## 사전 요구사항

| 프로젝트 타입 | 필요 도구 | 설치 |
|--------------|----------|------|
| Java/Maven | Apache Maven | https://maven.apache.org |
| Java/Gradle | Gradle Wrapper (`gradlew`) | 프로젝트 내 포함 |
| Go | `cyclonedx-gomod` | `go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest` |
| C#/.NET | .NET SDK + dotnet-CycloneDX | `dotnet tool install --global CycloneDX` |
| 그 외 모든 타입 | Node.js + cdxgen | `npm install -g @cyclonedx/cdxgen` |

> CocoaPods는 `Podfile.lock`을 직접 파싱하므로 별도 도구가 필요 없습니다.

## 설치

[Releases](../../releases) 페이지에서 플랫폼에 맞는 바이너리를 다운로드하거나, 직접 빌드합니다.

### 직접 빌드

취약점이 있는 mod 교체(한번만 실행)
```bash
go get golang.org/x/crypto@v0.52.0
go get golang.org/x/net@v0.55.0
go mod tidy
```

```bash
# Go 1.21 이상 필요
git clone <repo>
cd dtrack-submit
go build -o dtrack-submit .

# 전체 플랫폼 크로스컴파일 (dist/ 폴더에 4개 바이너리 생성)
make build-all
```

## 사용법

### 가장 간단한 방법

실행 파일 옆에 `config.json`을 두면 디렉토리만 넘기면 됩니다.

```bash
dtrack-submit ./myproject
```

`--server`와 `--config`가 모두 없으면 현재 디렉토리 → 실행 파일 위치 순서로 `config.json`을 자동 탐색합니다.

### CLI 파라미터

```bash
dtrack-submit [dir] [flags]
```

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `--server` | Dependency-Track 서버 URL | (필수) |
| `--api-key` | Dependency-Track API 키 | (필수) |
| `--config` | JSON 설정 파일 경로 | 자동 탐색 |
| `--project` | 프로젝트 이름 | 자동 감지 |
| `--version` | 프로젝트 버전 | 자동 감지 |
| `--dir` | 스캔 디렉토리 (positional arg로도 지정 가능) | 현재 디렉토리 |

**설정 우선순위:** CLI 플래그 > JSON config > 자동 감지

### JSON 설정 파일

`config.json`은 `--config`로 명시하거나 자동 탐색됩니다.  
API 키가 담기므로 **`.gitignore`에 추가**하세요.

**단일 프로젝트:**
```json
{
  "server": "http://localhost:8080",
  "api_key": "odt_xxxxxxxxxxxxxxxxxxxxxx",
  "project": "MyApp",
  "version": "1.0.0"
}
```

**모노레포 — 서브 디렉토리별 이름/버전 지정:**
```json
{
  "server": "http://localhost:8080",
  "api_key": "odt_xxxxxxxxxxxxxxxxxxxxxx",
  "projects": {
    "api":       { "name": "pgTelemetry-API",     "version": "1.0.0" },
    "dashboard": { "name": "pgTelemetry-Web",     "version": "0.1.0" },
    "sdk-cpp":   { "name": "pgTelemetry-SDK-CPP" }
  }
}
```

`projects` 맵의 키는 서브 디렉토리 이름입니다. 지정하지 않은 서브 디렉토리는 `루트이름/서브디렉토리` 형식으로 자동 생성됩니다.

## 사용 예시

```bash
# config.json 자동 로딩 + 디렉토리 지정
dtrack-submit ./myproject

# config 파일 명시
dtrack-submit --config /etc/dtrack/config.json ./myproject

# 모든 값을 플래그로 직접 지정
dtrack-submit --server http://localhost:8080 --api-key odt_xxx ./myproject

# 이름/버전 오버라이드
dtrack-submit --server http://localhost:8080 --api-key odt_xxx --project MyApp --version 2.1.0 ./myproject

# config 일부를 CLI로 덮어쓰기
dtrack-submit --config config.json --version 1.2.3 ./myproject
```

## 실행 결과 예시

**단일 프로젝트:**
```
→ Scanning D:\Sources\pgEDR\pgEDR_Console
  [npm] pgedr-console @ 0.1.0
  Generator: @cyclonedx/cyclonedx-npm
→ Generating SBOM...
  Done (1425.4 KB, specVersion: 1.6)
→ Uploading...
→ Waiting for analysis...
✓ Components: 653  Vulnerabilities: 7 (Critical:2 High:4 Medium:1 Low:0)
```

**모노레포:**
```
→ Scanning D:\Sources\pgTelemetry
  Found 3 sub-projects

  [go] pgTelemetry/api @ 1.26.2
  Generator: cdxgen
→ Generating SBOM...
  Done (332.8 KB, specVersion: 1.6)
→ Uploading...
→ Waiting for analysis...
✓ Components: 42  Vulnerabilities: 0

  [npm] pgTelemetry/dashboard @ 0.1.0
  Generator: @cyclonedx/cyclonedx-npm
→ Generating SBOM...
  Done (972.1 KB, specVersion: 1.6)
→ Uploading...
→ Waiting for analysis...
✓ Components: 318  Vulnerabilities: 3 (Critical:0 High:2 Medium:1 Low:0)

  [cpp] pgTelemetry/sdk-cpp @ 0.0.0
  Generator: cdxgen (swift)
→ Generating SBOM...
  Done (1.3 KB, specVersion: 1.6)
→ Uploading...
→ Waiting for analysis...
✓ Components: 2  Vulnerabilities: 0
```

## MCP 서버

`dtrack-mcp-server`는 Dependency-Track 기능을 [Model Context Protocol](https://modelcontextprotocol.io)로 노출하는 별도 실행 파일입니다. Claude Desktop, Claude Code 등 MCP 클라이언트에서 자연어로 SBOM 등록·취약점 조회를 수행할 수 있습니다.

CLI와 동일한 프로젝트 타입 감지·SBOM 생성 로직을 사용하며, 전용 도구(`syft`, `cyclonedx-gomod` 등)가 없으면 lockfile을 직접 파싱해 PURL 포함 BOM을 생성합니다.

### 설치

Windows는 [Releases](../../releases)의 NSIS 설치 파일(`dtrack-submit-setup-*.exe`)로 CLI와 MCP 서버를 함께 설치합니다. 설치 중 **Dependency-Track 서버 주소와 API 키를 입력**하면 설정 파일(`~/.dtrack.json`, `설치폴더\config.json`)에 자동 저장되고, 설치 폴더가 PATH에 등록됩니다. 업그레이드 시 API 키를 빈칸으로 두면 기존 설정이 유지됩니다.

Linux는 `dtrack-mcp-server-linux-amd64` 바이너리를 받아 PATH에 두고, 아래 환경 변수 또는 설정 파일로 접속 정보를 지정합니다.

### 제공 도구

| 도구 | 설명 |
|------|------|
| `dtrack_submit_project` | 디렉토리를 자동 감지해 SBOM 생성 후 등록·분석 (CLI와 동일 로직) |
| `dtrack_upload_bom` | 현재 디렉토리의 SBOM을 생성해 업로드 (syft/lockfile) |
| `dtrack_list_projects` | 등록된 프로젝트 목록 조회 |
| `dtrack_get_findings` | 프로젝트의 취약점 목록 조회 (severity 필터 가능) |
| `dtrack_get_project_metrics` | 취약점 통계·위험 점수 조회 |
| `dtrack_get_component_remediation` | 특정 컴포넌트의 취약점·수정 버전 조회 |
| `dtrack_generate_report` | 패키지별 그룹화 취약점 리포트 생성 |
| `dtrack_check_update` | 최신 릴리스 확인 및 자가 업데이트 |

### 설정

Windows 설치 파일을 쓰면 설치 중 입력한 값이 설정 파일로 저장되므로 별도 설정이 필요 없습니다. 직접 지정하려면 환경 변수 또는 설정 파일을 사용합니다 (우선순위: 환경 변수 > 설정 파일).

| 환경 변수 | 설명 |
|-----------|------|
| `DTRACK_URL` | Dependency-Track 서버 URL (기본 `http://localhost:8080`) |
| `DTRACK_API_KEY` | API 키 |
| `DTRACK_CONFIG` | 설정 파일 경로 (생략 시 `./dtrack.json`, `~/.dtrack.json` 탐색) |
| `DTRACK_NO_UPDATE_CHECK` | `1`이면 시작 시 업데이트 확인 비활성화 |

설정 파일 예시는 `dtrack-mcp-server --example-config`로 출력할 수 있습니다.

### MCP 클라이언트 등록

설치 파일이 클라이언트에 서버를 자동 등록하지는 않으므로, 이 단계는 직접 해야 합니다. 설치 시 설정 파일이 저장됐다면 접속 정보(`-e`/`env`)는 생략해도 됩니다.

Claude Code:

```bash
# 설정 파일이 이미 저장된 경우
claude mcp add dependency-track dtrack-mcp-server

# 접속 정보를 직접 지정 (설정 파일보다 우선)
claude mcp add dependency-track dtrack-mcp-server \
  -e DTRACK_URL=http://localhost:8080 \
  -e DTRACK_API_KEY=odt_xxxxxxxxxxxxxxxxxxxxxx
```

Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "dependency-track": {
      "command": "dtrack-mcp-server",
      "env": {
        "DTRACK_URL": "http://localhost:8080",
        "DTRACK_API_KEY": "odt_xxxxxxxxxxxxxxxxxxxxxx"
      }
    }
  }
}
```

### 자가 업데이트

서버는 시작 시 백그라운드로 최신 릴리스를 확인하고(6시간마다, stderr로만 알림), 새 버전이 있으면 안내합니다. 클라이언트에서 `dtrack_check_update`를 호출하면 신버전을 확인하고, 동의 시 실제로 교체합니다. 교체된 바이너리는 **MCP 서버를 재시작한 뒤** 적용됩니다. (`DTRACK_NO_UPDATE_CHECK=1`로 시작 시 확인을 끌 수 있습니다.)

## API 키 발급

1. Dependency-Track 관리자 콘솔 접속 (`http://<server>`)
2. **Administration → Access Management → Teams** 이동
3. 팀 선택 (또는 새 팀 생성) → **API Keys** 탭
4. **+** 버튼으로 키 생성

## 처리 흐름

```
1. Config 로딩 (config.json 자동 탐색 → CLI 플래그로 병합)
2. 디렉토리 스캔 → 프로젝트 타입 감지 (모노레포 시 하위 디렉토리 스캔)
3. manifest에서 이름/버전 자동 추출
4. SBOM 생성 (언어별 전용 도구 → cdxgen 폴백)
5. specVersion을 1.6으로 강제 조정 (Dependency-Track 호환)
6. Dependency-Track에 프로젝트 생성 (이미 있으면 기존 UUID 재사용)
7. SBOM 업로드 → 분석 완료 대기 (polling)
8. 결과 출력 (컴포넌트 수, 취약점 수)
```
