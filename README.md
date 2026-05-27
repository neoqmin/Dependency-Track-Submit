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
