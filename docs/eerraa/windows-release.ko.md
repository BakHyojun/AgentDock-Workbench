# 이 fork의 Windows 릴리즈 등록

`main`은 소스코드이고, GitHub **Release**는 특정 소스 버전의 설치파일을 내려받는 페이지다. main에 코드를 push해도 설치파일은 자동 등록되지 않는다. 이 fork는 GitHub Actions가 꺼져 있으므로 로컬 clean-clone 빌드를 등록한다.

## 사용자가 웹에서 등록하는 순서

1. 새 버전으로 소스 수정과 검증을 마치고 fork의 main에 반영한다. 게시된 버전의 태그·설치파일은 덮어쓰지 않는다.
2. 새 일반 clone에서 Windows x64 Setup을 빌드한다. build-report의 전체 commit과 실제 실행파일의 commit이 일치하는지 확인한다. 태그 `v<버전>`은 **실제 빌드 source commit**을 가리켜야 한다. 빌드 뒤 main에 문서 커밋이 추가됐다면 최신 main을 임의로 태그하지 않는다.
3. GitHub [Releases](https://github.com/eerraa/AgentDock-Workbench/releases) → **Draft a new release** → 빌드 commit에 만든 태그를 선택한다.
4. 버전 제목과 한국어 변경 설명, 통과/실패/미실행 검사, 미서명 여부를 적는다. **Attach binaries** 영역에 아래 6개 파일을 첨부한다.
   - `AgentDockSetup-amd64.exe`와 `.sha256`
   - `build-report.json`과 `.sha256`
   - `verification-scope.json`과 `.sha256`
5. **Save draft**로 보관하고 업로드된 파일을 다시 내려받아 크기와 SHA-256을 원본과 비교한다. Draft는 일반 사용자에게 공개되지 않는다.
6. 실제 설치 수용 전인 후보를 사용자의 요청으로 배포할 때는 **This is a pre-release**를 선택한다. 정식 배포는 기존 isolated Windows/Linux 검증 관문의 증거가 있어야 하며 이를 생략하거나 metadata에 통과로 쓰지 않는다.
7. **Publish release**를 누른다. 공개 페이지의 다운로드로 다시 내려받아 SHA-256을 확인하고 링크를 공유한다. 소스 ZIP/TAR 대신 Assets의 EXE를 받도록 안내한다.

1.1.8200은 위 과정으로 공개하는 Pre-release다. `latest` 정식 릴리즈 선택은 유지하며 이 후보를 정식 수용 완료로 표시하지 않는다. 운영 PC에서 Setup을 시험하거나 Actions 차단을 해제하는 절차가 아니다.

## CLI로 수행하는 동일한 과정

아래는 새 버전용 예시다. 이미 게시된 `v1.1.8200`에는 재실행하지 않는다. `$buildCommit`, `$assetDirectory`를 실제 검증 결과로 채운다. 태그 이동·force push·`--clobber`는 사용하지 않는다.

```powershell
git tag -a v<새버전> $buildCommit -m 'Windows x64 build [skip ci]'
git push origin refs/tags/v<새버전>
gh release create v<새버전> --repo eerraa/AgentDock-Workbench --verify-tag --draft --prerelease --latest=false --title 'AgentDock Workbench <새버전> — Windows x64' --notes-file docs/releases/v<새버전>.md
# 검증한 위 6개 파일을 명시적으로 upload한다.
gh release upload v<새버전> <파일경로들> --repo eerraa/AgentDock-Workbench
gh release download v<새버전> --repo eerraa/AgentDock-Workbench --dir <새임시폴더>
# 각 파일의 원본/다운로드 해시가 같은지 확인한 뒤 공개한다.
gh release edit v<새버전> --repo eerraa/AgentDock-Workbench --draft=false --prerelease=true --latest=false
# 별도 새 폴더로 공개 자산을 다시 다운로드·검증한다.
```

GitHub 공식 설명: [릴리즈 등록/수정](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository), [gh release create](https://cli.github.com/manual/gh_release_create). fork의 소스·검증·업데이트 불변식은 [fork 맵](fork-map.ko.md)이 기준이다.
