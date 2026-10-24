# FrpcShell 构建脚本
# 产物：build/FrpcShell.exe（windowsgui 子系统，无控制台窗口）

.PHONY: build test vet clean

build:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o build/FrpcShell.exe .

test:
	go vet ./...
	go test ./...

clean:
	rm -rf build
