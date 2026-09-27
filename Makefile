PREFIX ?= $(HOME)/.local

QS_PLUGIN_DIR ?= $(HOME)/.config/omarchy/plugins/dev.omavm.bar

.PHONY: build build-cli build-gui test vet fmt fmt-check check run run-gui clean install uninstall install-quickshell-plugin uninstall-quickshell-plugin

build: build-cli build-gui

build-cli:
	go build -o bin/omavm ./cmd/omavm

build-gui:
	mkdir -p build/gui bin
	cd build/gui && qmake6 ../../omavm-gui.pro && $(MAKE)
	cp build/gui/omavm-gui bin/omavm-gui.new
	mv bin/omavm-gui.new bin/omavm-gui

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .
	-clang-format -i gui/*.cpp gui/*.h

fmt-check:
	@fmted=$$(gofmt -l .); \
	if [ -n "$$fmted" ]; then \
		echo "gofmt needed on:"; echo "$$fmted"; exit 1; \
	fi

check: fmt-check vet test build

run: build-cli
	./bin/omavm $(ARGS)

run-gui: build-cli build-gui
	./bin/omavm-gui

clean:
	rm -rf bin build

# User-level install (no root): binaries on PATH, a .desktop entry so
# omavm-gui shows up in the app launcher/taskbar with its own icon, and
# icon caches refreshed best-effort. See README's "Logs" section for the
# separate, optional, explicitly-privileged step to provision
# /var/log/omavm instead of the default user-level log location.
install: build
	install -Dm755 bin/omavm $(PREFIX)/bin/omavm
	install -Dm755 bin/omavm-gui $(PREFIX)/bin/omavm-gui
	install -Dm644 data/dev.omavm.app.desktop $(PREFIX)/share/applications/dev.omavm.app.desktop
	install -Dm644 data/icons/dev.omavm.app.svg $(PREFIX)/share/icons/hicolor/scalable/apps/dev.omavm.app.svg
	-update-desktop-database $(PREFIX)/share/applications 2>/dev/null
	-gtk-update-icon-cache $(PREFIX)/share/icons/hicolor 2>/dev/null

uninstall:
	rm -f $(PREFIX)/bin/omavm $(PREFIX)/bin/omavm-gui
	rm -f $(PREFIX)/share/applications/dev.omavm.app.desktop
	rm -f $(PREFIX)/share/icons/hicolor/scalable/apps/dev.omavm.app.svg
	-update-desktop-database $(PREFIX)/share/applications 2>/dev/null

# Copies the Omarchy Quickshell bar-widget plugin into place — nothing
# more. It deliberately does NOT run `omarchy-shell shell rescanPlugins`
# or `omarchy plugin enable`: Omarchy's own trust model lands third-party
# plugins disabled so a human reviews the QML before it runs unsandboxed
# inside the already-live desktop shell, and this target respects that.
install-quickshell-plugin:
	mkdir -p $(QS_PLUGIN_DIR)
	cp -r contrib/dev.omavm.bar/. $(QS_PLUGIN_DIR)/
	@echo "Copied to $(QS_PLUGIN_DIR). Review the QML, then:"
	@echo "  omarchy-shell shell rescanPlugins && omarchy plugin enable dev.omavm.bar"

uninstall-quickshell-plugin:
	-omarchy plugin remove dev.omavm.bar
	rm -rf $(QS_PLUGIN_DIR)
