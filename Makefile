PLUGIN_DEST = $(HOME)/.config/zellij/plugins/trailboss.wasm
DAEMON_DEST = $(HOME)/.local/bin/trailboss
NVIM_DEST   = $(HOME)/.config/nvim/lua/trailboss.lua
DEV_SOURCE  = /tmp/trailboss-dev.jsonl
DEV_CONFIG  = $(CURDIR)/config.dev.toml

.PHONY: build build-plugin build-daemon install install-plugin install-daemon install-nvim dev clean

build: build-plugin build-daemon

build-plugin:
	cd plugin && cargo build --release

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build-daemon:
	cd daemon && go build -ldflags "-X main.version=$(VERSION)" -o ../bin/trailboss ./cmd/trailboss

install: install-plugin install-daemon install-nvim

install-plugin: build-plugin
	mkdir -p $(dir $(PLUGIN_DEST))
	cp plugin/target/wasm32-wasip1/release/trailboss.wasm $(PLUGIN_DEST)

install-daemon: build-daemon
	mkdir -p $(dir $(DAEMON_DEST))
	cp bin/trailboss $(DAEMON_DEST)

install-nvim:
	ln -sf $(CURDIR)/nvim/lua/trailboss.lua $(NVIM_DEST)

dev: install-daemon
	test -f $(DEV_CONFIG) || cp config.dev.example $(DEV_CONFIG)
	touch $(DEV_SOURCE)
	-pkill -f "trailboss daemon.*$(DEV_CONFIG)"
	sleep 0.2
	./bin/trailboss daemon -c $(DEV_CONFIG)

clean:
	cd plugin && cargo clean
	rm -rf bin/
