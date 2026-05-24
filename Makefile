PLUGIN_DEST = $(HOME)/.config/zellij/plugins/trailboss.wasm
DAEMON_DEST = $(HOME)/.local/bin/trailboss
NVIM_DEST   = $(HOME)/.config/nvim/lua/trailboss.lua
CONFIG_DEST = $(HOME)/.config/trailboss/config.toml
SOURCE_DEST = $(HOME)/.local/share/trailboss/comments.jsonl
DEV_SOURCE  = /tmp/trailboss-dev.jsonl
DEV_CONFIG  = $(CURDIR)/config.dev.toml

.PHONY: build build-plugin build-cli build-daemon install install-plugin install-cli install-daemon install-nvim install-config dev clean

build: build-plugin build-cli

build-plugin:
	cd plugin && cargo build --release

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build-cli:
	cd daemon && go build -ldflags "-X main.version=$(VERSION)" -o ../bin/trailboss ./cmd/trailboss

build-daemon: build-cli

install: install-plugin install-cli install-nvim install-config

install-plugin: build-plugin
	mkdir -p $(dir $(PLUGIN_DEST))
	cp plugin/target/wasm32-wasip1/release/trailboss.wasm $(PLUGIN_DEST)

install-cli: build-cli
	mkdir -p $(dir $(DAEMON_DEST))
	rm -f $(DAEMON_DEST)
	cp bin/trailboss $(DAEMON_DEST)

install-daemon: install-cli

install-nvim:
	mkdir -p $(dir $(NVIM_DEST))
	ln -sf $(CURDIR)/nvim/lua/trailboss.lua $(NVIM_DEST)

install-config:
	mkdir -p $(dir $(CONFIG_DEST))
	test -f $(CONFIG_DEST) || cp daemon/config.default.toml $(CONFIG_DEST)
	mkdir -p $(dir $(SOURCE_DEST))
	touch $(SOURCE_DEST)

dev: install-cli
	test -f $(DEV_CONFIG) || cp config.dev.example $(DEV_CONFIG)
	touch $(DEV_SOURCE)
	-pkill -f "trailboss daemon.*$(DEV_CONFIG)"
	sleep 0.2
	./bin/trailboss daemon -c $(DEV_CONFIG)

clean:
	cd plugin && cargo clean
	rm -rf bin/
