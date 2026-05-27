# dtrack-submit

Dependency-Track에 프로젝트 SBOM을 자동으로 등록하는 멀티플랫폼 CLI 툴.

프로젝트 디렉토리를 지정하면 언어/빌드 시스템을 자동으로 감지하여 CycloneDX SBOM을 생성하고 Dependency-Track 서버에 업로드합니다.

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

언어별 전용 도구가 설치되지 않은 경우 **cdxgen으로 자동 폴백**합니다.

## 사전 요구사항

### 필수
- 스캔할 프로젝트의 빌드 도구 (아래 중 해당하는 것)

| 프로젝트 타입 | 필요 도구 | 설치 |
|--------------|----------|------|
| Java/Maven | Apache Maven | https://maven.apache.org |
| Java/Gradle | Gradle (또는 Gradle Wrapper) | 프로젝트 내 `gradlew` 사용 |
| Go | `cyclonedx-gomod` | `go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest` |
| C#/.NET | .NET SDK + dotnet-CycloneDX | `dotnet tool install --global CycloneDX` |
| C++ | Node.js + cdxgen | `npm install -g @cyclonedx/cdxgen` |
| Node.js | Node.js + npx | https://nodejs.org |

### 폴백 (모든 타입에 사용 가능)
```bash
npm install -g @cyclonedx/cdxgen
```

## 설치

[Releases](../../releases) 페이지에서 플랫폼에 맞는 바이너리를 다운로드하거나, 직접 빌드합니다.

### 직접 빌드

```bash
# Go 1.21 이상 필요
git clone <repo>
cd dtrack-submit
go build -o dtrack-submit .

# 전체 플랫폼 크로스컴파일
make build-all
```

## 사용법

### CLI 파라미터

```bash
dtrack-submit --server <URL> --api-key <KEY> --dir <프로젝트경로>
```

#### 옵션

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `--server` | Dependency-Track 서버 URL | (필수) |
| `--api-key` | Dependency-Track API 키 | (필수) |
| `--dir` | 스캔할 프로젝트 디렉토리 | 현재 디렉토리 |
| `--project` | 프로젝트 이름 (자동 감지 우선) | 자동 |
| `--version` | 프로젝트 버전 (자동 감지 우선) | 자동 |
| `--config` | JSON 설정 파일 경로 | — |

### JSON 설정 파일

`--config` 플래그로 JSON 파일을 지정할 수 있습니다. CLI 플래그가 JSON 값보다 우선합니다.

```bash
dtrack-submit --config config.json
```

**config.json 예시 — 단일 프로젝트:**
```json
{
  "server": "http://localhost:8080",
  "api_key": "odt_xxxxxxxxxxxxxxxxxxxxxx",
  "dir": "/path/to/project",
  "project": "MyApp",
  "version": "1.0.0"
}
```

**config.json 예시 — 모노레포 (서브 디렉토리별 이름/버전 지정):**
```json
{
  "server": "http://localhost:8080",
  "api_key": "odt_xxxxxxxxxxxxxxxxxxxxxx",
  "projects": {
    "api": { "name": "pgTelemetry-API", "version": "1.0.0" },
    "dashboard": { "name": "pgTelemetry-Web", "version": "0.1.0" },
    "sdk-cpp": { "name": "pgTelemetry-SDK-CPP" }
  }
}
```

`projects` 맵의 키는 서브 디렉토리 이름입니다. 지정하지 않은 서브 디렉토리는 자동 감지된 이름을 사용합니다.

## 예시

### 기본 사용

```bash
# Node.js 프로젝트
dtrack-submit --server http://localhost:8080 --api-key odt_xxx --dir ./my-frontend

# Go 프로젝트
dtrack-submit --server http://localhost:8080 --api-key odt_xxx --dir ./my-service

# C# 프로젝트 (이름/버전 직접 지정)
dtrack-submit --server http://localhost:8080 --api-key odt_xxx --dir ./MyApp --project MyApp --version 2.1.0

# 현재 디렉토리 스캔
dtrack-submit --server http://localhost:8080 --api-key odt_xxx
```

### JSON 설정 파일 사용

```bash
dtrack-submit --config config.json

# JSON 설정에서 일부 값만 CLI로 덮어쓰기
dtrack-submit --config config.json --version 1.2.3
```

### 실행 결과 예시

```
→ Scanning /path/to/my-frontend
  Detected: npm
  Project:  my-frontend @ 0.1.0
  Generator: @cyclonedx/cyclonedx-npm
→ Generating SBOM...
  Done (dtrack-bom.json, 1425.4 KB)
→ Creating project in Dependency-Track...
  UUID: 4995ea72-3aad-4ccd-8d77-69a88624a69f
→ Uploading SBOM...
→ Waiting for analysis to complete...

✓ Done!
  Components:      653
  Vulnerabilities: 7 (Critical:2 High:4 Medium:1 Low:0)
  Dashboard: http://localhost:8080
```

## API 키 발급

1. Dependency-Track 관리자 콘솔 접속 (`http://<server>`)
2. **Administration → Access Management → Teams** 이동
3. 팀 선택 (또는 새 팀 생성) → **API Keys** 탭
4. **+** 버튼으로 키 생성

## 처리 흐름

```
1. Config 로딩 (JSON 파일 + CLI 플래그 병합)
2. 디렉토리 스캔 → 프로젝트 타입 감지
3. manifest 파일에서 이름/버전 자동 추출
4. SBOM 생성 (언어별 전용 도구 → cdxgen 폴백)
5. Dependency-Track에 프로젝트 생성 (없으면 신규, 있으면 재사용)
6. SBOM 업로드
7. 분석 완료 대기 (polling)
8. 결과 출력 (컴포넌트 수, 취약점 수)
```

## 빌드

```bash
# 현재 플랫폼용
go build -o dtrack-submit .

# 전체 플랫폼 (Makefile)
make build-all
# dist/ 폴더에 4개 바이너리 생성
```
