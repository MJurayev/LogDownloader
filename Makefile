VERSION ?= 0.0.1
GOOS ?= linux
GOARCH ?= amd64

.PHONY: frontend build package-rpm package-deb clean

frontend:
	cd frontend && npm ci && npm run build
	rm -rf backend/web/dist
	cp -r frontend/dist backend/web/dist

build: frontend
	cd backend && CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags="-s -w" -o ../build/logdownloader-$(GOOS)-$(GOARCH) ./cmd

package-rpm: build
	VERSION=$(VERSION) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		nfpm package --packager rpm --target build/

package-deb: build
	VERSION=$(VERSION) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		nfpm package --packager deb --target build/

clean:
	rm -rf build/ backend/web/dist
