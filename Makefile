.PHONY: fmt test build e2e run-controlplane desktop-qt-configure desktop-qt-build

fmt:
	$(MAKE) -C packages/contracts/go fmt
	$(MAKE) -C packages/runtime/go fmt
	$(MAKE) -C clients/linux-agent fmt
	$(MAKE) -C clients/windows-agent fmt
	$(MAKE) -C services/controlplane fmt
	$(MAKE) -C services/relay fmt
	$(MAKE) -C clients/linux-cli fmt
	$(MAKE) -C clients/windows-cli fmt

test:
	$(MAKE) -C packages/contracts/go test
	$(MAKE) -C packages/runtime/go test
	$(MAKE) -C clients/linux-agent test
	$(MAKE) -C clients/windows-agent test
	$(MAKE) -C services/controlplane test
	$(MAKE) -C services/relay test
	$(MAKE) -C clients/linux-cli test
	$(MAKE) -C clients/windows-cli test

build:
	mkdir -p dist
	$(MAKE) -C clients/linux-agent build
	$(MAKE) -C clients/windows-agent build
	$(MAKE) -C services/controlplane build
	$(MAKE) -C services/relay build
	$(MAKE) -C clients/linux-cli build
	$(MAKE) -C clients/windows-cli build

e2e:
	bash scripts/e2e_smoke.sh

run-controlplane:
	$(MAKE) -C services/controlplane run-controlplane

desktop-qt-configure:
	cmake -S clients/desktop-qt -B build/desktop-qt

desktop-qt-build: desktop-qt-configure
	cmake --build build/desktop-qt -j
