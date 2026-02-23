VERSION := 1.0.0
BINARY  := diskbench
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all clean build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY)

all: build/$(BINARY)-linux-amd64 \
     build/$(BINARY)-linux-arm64 \
     build/$(BINARY)-darwin-amd64 \
     build/$(BINARY)-darwin-arm64 \
     build/$(BINARY)-windows-amd64.exe

build/$(BINARY)-linux-amd64:
	@mkdir -p build
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $@

build/$(BINARY)-linux-arm64:
	@mkdir -p build
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $@

build/$(BINARY)-darwin-amd64:
	@mkdir -p build
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $@

build/$(BINARY)-darwin-arm64:
	@mkdir -p build
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $@

build/$(BINARY)-windows-amd64.exe:
	@mkdir -p build
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $@

clean:
	rm -rf build/ $(BINARY)
