BINARY  := ferryman
PKG     := .
DIST    := dist
VERSION ?= dev

# CGO 비활성화 + vendor 사용으로 airgap 에서도 정적 바이너리를 산출한다.
GOFLAGS := -mod=vendor -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
BUILD    = CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)'

.PHONY: all build build-all build-linux build-windows build-darwin \
        vendor test vet clean run

all: build

## build: 현재 플랫폼용 바이너리
build:
	$(BUILD) -o $(BINARY) $(PKG)

## build-all: 세 플랫폼(amd64/arm64) 전체 산출
build-all: build-linux build-windows build-darwin

build-linux:
	GOOS=linux   GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-linux-amd64   $(PKG)
	GOOS=linux   GOARCH=arm64 $(BUILD) -o $(DIST)/$(BINARY)-linux-arm64   $(PKG)

build-windows:
	GOOS=windows GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-windows-amd64.exe $(PKG)

build-darwin:
	GOOS=darwin  GOARCH=amd64 $(BUILD) -o $(DIST)/$(BINARY)-darwin-amd64  $(PKG)
	GOOS=darwin  GOARCH=arm64 $(BUILD) -o $(DIST)/$(BINARY)-darwin-arm64  $(PKG)

## vendor: 의존성을 vendor/ 로 고정 (airgap 재빌드용)
vendor:
	go mod tidy
	go mod vendor

## test: 단위 테스트
test:
	go test $(GOFLAGS) ./...

## vet: 정적 분석
vet:
	go vet $(GOFLAGS) ./...

## run: 로컬 웹 서버 실행
run: build
	./$(BINARY) serve

## clean: 산출물 제거
clean:
	rm -rf $(BINARY) $(DIST)
