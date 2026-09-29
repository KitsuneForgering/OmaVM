PREFIX ?= $(HOME)/.local

QS_PLUGIN_DIR ?= $(HOME)/.config/omarchy/plugins/dev.omavm.bar

.PHONY: build build-cli build-gui test vet fmt fmt-check check run run-gui clean install uninstall uninstall-environments install-quickshell-plugin uninstall-quickshell-plugin test-display test-terminal test-backend test-qml

build: build-cli build-gui

build-cli:
	go build -o bin/omavm ./cmd/omavm

build-gui:
	rm -rf build/gui
	mkdir -p build/gui bin
	cd build/gui && qmake6 ../../omavm-gui.pro && $(MAKE)
	cp build/gui/omavm-gui bin/omavm-gui.new
	mv bin/omavm-gui.new bin/omavm-gui

test: test-display test-terminal test-backend test-qml
	go test ./...

test-display:
	mkdir -p build/display-tests
	cd build/display-tests && qmake6 ../../gui/tests/displayclient_test.pro && $(MAKE)
	./build/display-tests/displayclient_test
	cd build/display-tests && qmake6 ../../gui/tests/displayview_test.pro -o Makefile.view && $(MAKE) -f Makefile.view
	./build/display-tests/displayview_test

test-terminal:
	mkdir -p build/terminal-tests
	cd build/terminal-tests && qmake6 ../../gui/tests/terminal_test.pro && $(MAKE)
	./build/terminal-tests/terminal_test

test-backend:
	mkdir -p build/backend-tests
	cd build/backend-tests && qmake6 ../../gui/tests/backend_refresh_test.pro && $(MAKE)
	./build/backend-tests/backend_refresh_test

test-qml:
	mkdir -p build/qml-tests
	cd build/qml-tests && qmake6 ../../gui/tests/qml_test.pro && $(MAKE)
	QT_QPA_PLATFORM=offscreen ./build/qml-tests/qml_test

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
# omavm-gui shows up in the app launcher/taskbar with its own icon, icon
# caches refreshed best-effort, and the Quickshell bar-widget plugin
# staged (copied, never enabled — see install-quickshell-plugin below).
# See README's "Logs" section for the separate, optional,
# explicitly-privileged step to provision /var/log/omavm instead of the
# default user-level log location.
install: build install-quickshell-plugin
	install -Dm755 bin/omavm $(PREFIX)/bin/omavm
	install -Dm755 bin/omavm-gui $(PREFIX)/bin/omavm-gui
	install -Dm644 data/dev.omavm.app.desktop $(PREFIX)/share/applications/dev.omavm.app.desktop
	install -Dm644 data/icons/dev.omavm.app.svg $(PREFIX)/share/icons/hicolor/scalable/apps/dev.omavm.app.svg
	@# One icon per environment color, for launcher entries and the file
	@# manager (internal/desktop's ColorIcon).
	install -Dm644 -t $(PREFIX)/share/omavm/icons data/icons/colors/*.svg
	-update-desktop-database $(PREFIX)/share/applications 2>/dev/null
	@# ~/.local/share/icons/hicolor usually has no index.theme; the cache is
	@# optional there (icon lookup works without it), so only refresh it
	@# when the theme index exists instead of printing a spurious error.
	@if [ -f $(PREFIX)/share/icons/hicolor/index.theme ]; then \
		gtk-update-icon-cache -q $(PREFIX)/share/icons/hicolor || true; \
	fi

# Removes only the app itself (binaries, launcher entries, icons).
# Environments OmaVM created (Distrobox containers, Machine disks) are
# never touched here — uninstalling the manager must never silently
# destroy a user's containers or virtual disks. If any are still
# registered, this only warns and points at `uninstall-environments`.
uninstall:
	@if [ -x "$(PREFIX)/bin/omavm" ]; then \
		count=$$("$(PREFIX)/bin/omavm" list --json 2>/dev/null | grep -o '"name"' | wc -l); \
		if [ "$$count" -gt 0 ] 2>/dev/null; then \
			echo "Note: $$count OmaVM environment(s) still exist and were left untouched."; \
			echo "Run 'make uninstall-environments' to remove them too (asks for confirmation)."; \
		fi; \
	fi
	rm -f $(PREFIX)/bin/omavm $(PREFIX)/bin/omavm-gui
	rm -f $(PREFIX)/share/applications/dev.omavm.app.desktop
	rm -f $(PREFIX)/share/icons/hicolor/scalable/apps/dev.omavm.app.svg
	rm -rf $(PREFIX)/share/omavm/icons
	@# Per-environment launcher entries would run binaries that are gone;
	@# omavm writes them again on the next start after a reinstall.
	rm -f $(PREFIX)/share/applications/dev.omavm.env.*.desktop
	-update-desktop-database $(PREFIX)/share/applications 2>/dev/null

# Removes every Environment OmaVM still knows about (Boxes and
# Machines) through `omavm remove` — never raw podman/qemu commands
# here, so it gets the exact same Core validation and cleanup a human
# already gets from the CLI (Stop before disk removal for Machines,
# `distrobox rm --force` for Boxes — a Box's container is removed but
# files in the shared home directory are never touched). Deliberately
# separate from `install`/`uninstall`, never chained into either: opt-in
# only. CONFIRM=1 skips the interactive prompt for scripted/CI use.
uninstall-environments: build-cli
	@json=$$(./bin/omavm list --json 2>/dev/null); \
	if [ -z "$$json" ] || [ "$$json" = "[]" ]; then \
		echo "No OmaVM environments found."; \
		exit 0; \
	fi; \
	echo "This will permanently remove:"; \
	echo "  - Development Boxes: their containers (home directory files are not touched)"; \
	echo "  - Machines: their virtual disks and every snapshot"; \
	echo "Affected environments:"; \
	./bin/omavm list | sed 's/^/  /'; \
	if [ "$$CONFIRM" != "1" ]; then \
		printf "Type 'yes' to remove all of them: "; \
		read reply; \
		if [ "$$reply" != "yes" ]; then \
			echo "Aborted, nothing removed."; \
			exit 1; \
		fi; \
	fi; \
	printf '%s' "$$json" | grep -o '"name":"[^"]*"' | sed -E 's/"name":"(.*)"/\1/' | while IFS= read -r name; do \
		[ -z "$$name" ] && continue; \
		echo "Removing $$name..."; \
		./bin/omavm remove "$$name" || exit 1; \
	done

# Copies the Omarchy Quickshell bar-widget plugin into place — nothing
# more, and also run as part of `install` above. It deliberately does NOT
# run `omarchy-shell shell rescanPlugins` or `omarchy plugin enable`,
# even when chained from `install`: Omarchy's own trust model lands
# third-party plugins disabled so a human reviews the QML before it runs
# unsandboxed inside the already-live desktop shell, and this target
# respects that — copying the file is harmless, enabling it is not, so
# only the enable step stays manual.
#
# Removes the destination first so this is always an exact mirror of
# contrib/dev.omavm.bar/, never an old install's leftover files sitting
# alongside new ones: a plain `cp -r` only adds/overwrites, so a file
# renamed or removed from the plugin since a previous install would
# otherwise linger in $(QS_PLUGIN_DIR) forever and could still be picked
# up by the shell.
install-quickshell-plugin:
	rm -rf $(QS_PLUGIN_DIR)
	mkdir -p $(QS_PLUGIN_DIR)
	cp -r contrib/dev.omavm.bar/. $(QS_PLUGIN_DIR)/
	@echo "Copied to $(QS_PLUGIN_DIR). Review the QML, then:"
	@echo "  omarchy-shell shell rescanPlugins && omarchy plugin enable dev.omavm.bar"

uninstall-quickshell-plugin:
	-omarchy plugin remove dev.omavm.bar
	rm -rf $(QS_PLUGIN_DIR)
