# agentbox build and lifecycle. `make help` lists targets.

BINDIR  ?= $(HOME)/.local/bin
BIN      = agentbox
GOFLAGS ?=

# Desktop integration install locations (XDG user dirs; no root needed).
DESKTOPDIR ?= $(HOME)/.local/share/applications
ICONDIR    ?= $(HOME)/.local/share/icons/hicolor/256x256/apps
UNITDIR    ?= $(HOME)/.config/systemd/user

.PHONY: help build frontend test check fmt vet generate run stop logs deploy deployed \
	restart-daemon rollback clean install install-bin install-desktop install-service uninstall \
	bootstrap deps deps-build deps-desktop deps-speech config doctor deck

# Where a piper voice goes. agentbox looks here first (internal/speech/speech.go), so
# `[speech] enabled = true` is all the config a fresh machine needs afterwards.
VOICEDIR ?= $(HOME)/.local/share/piper-voices
VOICE    ?= en_US-lessac-high
VOICE_URL = https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/lessac/high
SUDO     ?= sudo

# The web UI is embedded with go:embed, so `go build` picks up whatever is in
# frontend/dist. dist/ is committed on purpose (a machine with no npm must still
# build agentbox), which means an edited .svelte file would otherwise produce a
# binary with the old UI and no warning.
FRONTEND_OUT  = frontend/dist/index.html
FRONTEND_SRCS = frontend/package.json frontend/index.html frontend/vite.config.js \
	frontend/svelte.config.js $(shell find frontend/src -type f 2>/dev/null)

help: ## list targets
	@awk -F':.*## ' '/^[a-z-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' Makefile

# ---------------------------------------------------------------- bootstrap
#
# One target for a machine that has never built agentbox: the toolchain it needs to
# compile, the desktop libraries the webview is, the two X utilities the driver
# leans on, a speech engine with a voice so the narration is not silent, and a
# config that turns speech on. It is separate from `install`, which only puts a
# built binary where the desktop can find it.
#
# Package installation needs root, and that is the one thing a Makefile should not
# be sneaky about: every privileged command goes through $(SUDO) so it is visible
# in the recipe, and `make doctor` tells you what is missing without installing
# anything at all.

bootstrap: deps config ## everything a fresh machine needs: packages, speech engine, config
	@echo
	@$(MAKE) --no-print-directory doctor
	@echo
	@echo "next: make build && make install && systemctl --user enable --now agentbox.service"

deps: deps-build deps-desktop deps-speech ## system packages and a speech engine

# The Go toolchain is deliberately NOT installed here: a Go version is a project
# decision (go.mod says 1.26), distro packages lag, and anyone building agentbox has
# already chosen how they get Go. This checks for it and says so.
deps-build: ## compiler, pkg-config, GTK4 + WebKitGTK headers, node/npm
	@command -v go >/dev/null 2>&1 || { \
		echo "go is not installed. agentbox needs $$(awk '/^go /{print $$2}' go.mod)+ from https://go.dev/dl/"; exit 1; }
	@if command -v apt-get >/dev/null 2>&1; then \
		echo "apt: build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev nodejs npm"; \
		$(SUDO) apt-get update && \
		$(SUDO) apt-get install -y build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev nodejs npm; \
	elif command -v dnf >/dev/null 2>&1; then \
		$(SUDO) dnf install -y gcc make pkgconf-pkg-config gtk4-devel webkitgtk6.0-devel nodejs npm; \
	elif command -v pacman >/dev/null 2>&1; then \
		$(SUDO) pacman -S --needed --noconfirm base-devel pkgconf gtk4 webkitgtk-6.0 nodejs npm; \
	else \
		echo "unknown package manager. agentbox needs: a C compiler, pkg-config,"; \
		echo "GTK4 and WebKitGTK 6.0 development headers, and node/npm."; exit 1; \
	fi

# wmctrl and xwininfo are what `agentbox drive` reads window geometry with when it is
# checked by hand, and ImageMagick's `import` is how a card is measured for the
# showcase. pipewire supplies pw-play, which is how speech reaches a sound device.
deps-desktop: ## X utilities the driver and the showcase use, and an audio player
	@if command -v apt-get >/dev/null 2>&1; then \
		$(SUDO) apt-get install -y wmctrl x11-utils imagemagick pipewire pipewire-pulse; \
	elif command -v dnf >/dev/null 2>&1; then \
		$(SUDO) dnf install -y wmctrl xorg-x11-utils ImageMagick pipewire pipewire-pulseaudio; \
	elif command -v pacman >/dev/null 2>&1; then \
		$(SUDO) pacman -S --needed --noconfirm wmctrl xorg-xwininfo imagemagick pipewire pipewire-pulse; \
	else \
		echo "install by hand: wmctrl, xwininfo, ImageMagick, pipewire"; \
	fi

# piper is the engine agentbox detects on its own: a line of text on stdin, raw PCM on
# stdout, and it stays alive between lines - which is the whole reason agentbox speaks
# in 70ms instead of 2.5s per sentence. It goes in its own venv rather than into
# the system interpreter, and the voice goes where agentbox already looks for one.
#
# Kokoro (24 kHz, markedly more natural) is the better voice and is NOT installed
# here: it is a 350 MB model and its own project. Point [speech] command at any
# engine that honours the same contract and agentbox will use it instead.
deps-speech: ## piper plus one voice, in ~/.local/share
	@if command -v piper >/dev/null 2>&1; then echo "piper: already installed at $$(command -v piper)"; \
	elif command -v pipx >/dev/null 2>&1; then pipx install piper-tts; \
	else \
		echo "installing piper into $(HOME)/.local/share/piper/venv"; \
		python3 -m venv $(HOME)/.local/share/piper/venv && \
		$(HOME)/.local/share/piper/venv/bin/pip install --quiet --upgrade pip piper-tts && \
		mkdir -p $(BINDIR) && ln -sf $(HOME)/.local/share/piper/venv/bin/piper $(BINDIR)/piper; \
	fi
	@if [ -f "$(VOICEDIR)/$(VOICE).onnx" ]; then echo "voice: $(VOICE) already present"; else \
		echo "fetching the $(VOICE) voice (~110 MB) into $(VOICEDIR)"; \
		mkdir -p $(VOICEDIR) && \
		curl -fsSL -o $(VOICEDIR)/$(VOICE).onnx      "$(VOICE_URL)/$(VOICE).onnx" && \
		curl -fsSL -o $(VOICEDIR)/$(VOICE).onnx.json "$(VOICE_URL)/$(VOICE).onnx.json"; \
	fi

# config writes a file only if there is none: a bootstrap that overwrote somebody's
# tuned configuration would be a bug, not a convenience. Everything agentbox has is
# optional and documented in docs/06-configuration.md; the one thing worth turning
# on out of the box is the voice, because silence looks like a broken install.
config: ## write ~/.config/agentbox/config.toml if it does not exist (speech on)
	@dir=$${XDG_CONFIG_HOME:-$(HOME)/.config}/agentbox; \
	if [ -f "$$dir/config.toml" ]; then echo "config: keeping $$dir/config.toml"; else \
		mkdir -p "$$dir"; \
		{ echo "# agentbox configuration. Every key is optional - the defaults are the full"; \
		  echo "# experience - so this file only records what differs. See"; \
		  echo "# docs/06-configuration.md for every knob and why its default is what it is."; \
		  echo; \
		  echo "[speech]"; \
		  echo "# Read the line an agent attaches to a card, after the earcon. With no"; \
		  echo "# command set, agentbox finds piper and picks the best voice it can see."; \
		  echo "enabled = true"; \
		  echo "prewarm = true      # load the voice at daemon start, so the first line is instant"; \
		  echo "# command = [\"$(HOME)/.local/bin/kokoro-say\"]   # a better voice, if you have it"; \
		  echo "# rate = 24000                                    # Kokoro's native rate"; \
		} > "$$dir/config.toml"; \
		echo "config: wrote $$dir/config.toml"; \
	fi

doctor: ## report what is present and what is missing, installing nothing
	@printf '%-22s %s\n' "go" "$$(command -v go >/dev/null 2>&1 && go version || echo MISSING)"
	@printf '%-22s %s\n' "npm" "$$(command -v npm >/dev/null 2>&1 && npm --version || echo 'missing (fine: frontend/dist is committed)')"
	@printf '%-22s %s\n' "gtk4 + webkitgtk" "$$(pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null && echo present || echo MISSING)"
	@printf '%-22s %s\n' "X display" "$$(test -n "$$DISPLAY" && echo "$$DISPLAY" || echo 'none (cards need X11; XTEST for agentbox drive)')"
	@printf '%-22s %s\n' "wmctrl / xwininfo" "$$(command -v wmctrl >/dev/null 2>&1 && command -v xwininfo >/dev/null 2>&1 && echo present || echo 'missing (only needed to measure windows by hand)')"
	@printf '%-22s %s\n' "ImageMagick import" "$$(command -v import >/dev/null 2>&1 && echo present || echo 'missing (only needed for showcase screenshots)')"
	@printf '%-22s %s\n' "speech engine" "$$(command -v kokoro-say 2>/dev/null || command -v piper 2>/dev/null || echo 'MISSING (make deps-speech)')"
	@printf '%-22s %s\n' "piper voice" "$$(ls $(VOICEDIR)/*.onnx $(HOME)/piper-voices/*.onnx 2>/dev/null | head -1 || echo 'none (not needed if [speech] command is set)')"
	@printf '%-22s %s\n' "audio player" "$$(command -v pw-play 2>/dev/null || command -v aplay 2>/dev/null || echo MISSING)"
	@printf '%-22s %s\n' "config" "$$(f=$${XDG_CONFIG_HOME:-$(HOME)/.config}/agentbox/config.toml; test -f $$f && echo $$f || echo 'none (defaults; speech off)')"
	@printf '%-22s %s\n' "installed binary" "$$(test -x $(BINDIR)/$(BIN) && $(BINDIR)/$(BIN) version | head -1 || echo 'not installed (make install)')"
	@printf '%-22s %s\n' "daemon" "$$($(BINDIR)/$(BIN) status 2>/dev/null | head -1 || echo 'not running')"
	@printf '%-22s %s\n' "systemd unit" "$$(systemctl --user is-enabled agentbox.service 2>/dev/null || echo 'not enabled (make install-service)')"

build: frontend ## build ./agentbox (rebuilds the web UI when its sources changed)
	go build $(GOFLAGS) -o $(BIN) ./cmd/agentbox

frontend: $(FRONTEND_OUT) ## rebuild frontend/dist if a source is newer

$(FRONTEND_OUT): $(FRONTEND_SRCS)
	@if command -v npm >/dev/null 2>&1; then \
		cd frontend && { [ -d node_modules ] || npm install; } && npm run build; \
	else \
		echo "npm not found: embedding the committed frontend/dist as is"; \
	fi

test: ## full suite with the race detector
	go test ./... -race -count=1

# The frontend's shared modules, on node's own runner - no framework, because a
# dist that is committed so a machine without npm can still build should not
# start needing npm to be tested. No node is not a failure for the same reason:
# it says so and moves on, exactly like the build does.
test-js: ## frontend module tests (skipped when node is absent)
	@if command -v node >/dev/null 2>&1; then \
		node --test "frontend/src/**/*.test.js"; \
	else \
		echo "node not found: skipping the frontend module tests"; \
	fi

fmt: ## fail when any file is not gofmt-clean
	@out=$$(gofmt -l cmd internal tools); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet: fmt ## gofmt check + go vet
	go vet ./...

check: vet test test-js ## everything CI would run

generate: ## regenerate earcon WAVs
	go run ./tools/genearcons internal/sound/assets

# run is the dev loop: it puts THIS working copy's build on the default instance.
# Since agentbox is deployed, that means displacing the daemon Boris's Claude sessions
# reach him through - the flock allows one per instance - so it says so rather than
# doing it quietly. `make restart-daemon` puts the deployed build back; or set
# AGENTBOX_INSTANCE to run beside it instead of over it.
run: build stop ## (re)start the daemon detached, from this working copy
	@if [ -z "$$AGENTBOX_INSTANCE" ] && systemctl --user is-enabled --quiet agentbox.service 2>/dev/null; then \
		echo 'note: this replaces the deployed daemon on the default instance.'; \
		echo '      `make restart-daemon` restores it; AGENTBOX_INSTANCE=dev runs beside it.'; \
	fi
	setsid ./$(BIN) daemon >/dev/null 2>&1 < /dev/null &
	@sleep 1 && ./$(BIN) status

# kill-daemons is the force half of stop, and it is fussy for two reasons that
# have each cost a session.
#
#   * `pkill -f "agentbox daemon"` matches the shell running this recipe, because
#     that string is IN the recipe. It has killed a session before.
#   * `pkill -x agentbox` matches the process NAME, which is right for the daemon and
#     WRONG for everything else called agentbox - every Claude session holds a
#     `agentbox mcp` child, and killing those takes agentbox's tools away from every
#     agent on the machine mid-task. It happened on the deploy that found this.
#
# So: ask for the processes named agentbox, then read each one's own command line and
# kill only the daemons. Nothing here can match this recipe, and no agent loses
# its tools because somebody deployed.
define kill-daemons
for p in $$(pgrep -x $(BIN) 2>/dev/null); do 	if tr '\0' ' ' < /proc/$$p/cmdline 2>/dev/null | grep -q ' daemon '; then 		kill $$p 2>/dev/null || true; 	fi; done
endef

stop: ## stop the daemon gracefully (agentbox quit), force any stragglers
	-./$(BIN) quit 2>/dev/null || true
	-@$(kill-daemons)

logs: ## tail the daemon event log
	./$(BIN) logs --follow

# deploy REPLACES whatever is at $(BINDIR)/$(BIN) and leaves the daemon running
# the new build. The order matters: stop the old daemon first, because a daemon
# whose file is replaced underneath it keeps serving the old code from an unlinked
# inode - the binary would be new and the behaviour would not. The build that was
# replaced is kept as $(BIN).prev so `make rollback` is one command.
#
# Two agents deploying at once is the CLAUDE.md trap this serializes, and it is
# flock rather than `agentbox sync lock` on purpose: FR83's locks live in the
# daemon's memory, and this recipe STOPS the daemon halfway through. A sync hold
# would vanish with it and hand the second agent a green light in the middle of
# the first one's install. The one resource the daemon cannot arbitrate is the
# daemon. The kernel can, so the kernel does; the wait says who holds it, because
# a deploy that looks hung is how an agent starts a second one.
DEPLOY_LOCK ?= $(BINDIR)/.$(BIN).deploy.lock
deploy: ## replace the deployed binary in $(BINDIR) and restart the daemon on it
	@install -d $(BINDIR)
	@if command -v flock >/dev/null 2>&1; then \
		flock -n $(DEPLOY_LOCK) true 2>/dev/null \
			|| echo "another deploy holds $(DEPLOY_LOCK) ($$(cat $(DEPLOY_LOCK).who 2>/dev/null || echo 'holder unknown')); waiting up to 15m"; \
		flock -w 900 $(DEPLOY_LOCK) $(MAKE) --no-print-directory deploy-locked \
			|| { echo "deploy: gave up waiting for the deploy lock"; exit 1; }; \
	else \
		echo "warning: no flock here, so nothing stops a second agent deploying at the same time"; \
		$(MAKE) --no-print-directory deploy-locked; \
	fi

deploy-locked: check build ## the deploy itself; take the lock through `make deploy` instead
	@echo "pid $$$$ started $$(date -Is)" > $(DEPLOY_LOCK).who
	@if ./$(BIN) version | grep -q dirty; then \
		echo "warning: deploying a dirty build (NFR14: commit first for a clean stamp)"; fi
	install -d $(BINDIR)
	@if [ -x $(BINDIR)/$(BIN) ]; then \
		echo "replacing $$($(BINDIR)/$(BIN) version 2>/dev/null || echo 'an unreadable build')"; \
		cp -p $(BINDIR)/$(BIN) $(BINDIR)/$(BIN).prev; \
	fi
	-$(BINDIR)/$(BIN) quit 2>/dev/null || true
	@sleep 0.5
	-@$(kill-daemons)
	install -m 0755 $(BIN) $(BINDIR)/$(BIN)
	@echo "installed  $$($(BINDIR)/$(BIN) version)"
	@$(MAKE) --no-print-directory restart-daemon
	@sleep 1.5; $(MAKE) --no-print-directory deployed

# restart-daemon brings the daemon back on whatever is now at $(BINDIR)/$(BIN),
# through systemd when the unit is enabled and as a detached process when it is
# not. The systemd branch is not a nicety: starting a detached daemon while the
# unit is enabled leaves the service inactive with an unmanaged daemon running, so
# every deploy would quietly orphan the unit - and the next `systemctl restart`
# would hit agentbox's "already running" path and go inactive again.
# It stops whatever is serving first, and has to: agentbox is single-instance by
# flock, so `systemctl restart` while an unmanaged daemon holds the lock hits the
# "already running" path, exits 0, and leaves the unit inactive again. deploy has
# already stopped by the time it gets here, so that quit is a no-op there.
restart-daemon: ## start the daemon on the installed binary, via systemd if enabled
	-@$(BINDIR)/$(BIN) quit 2>/dev/null || true
	@sleep 0.5
	-@$(kill-daemons)
	@if systemctl --user is-enabled --quiet agentbox.service 2>/dev/null; then \
		echo "restarting agentbox.service (the unit owns the daemon)"; \
		systemctl --user restart agentbox.service; \
	else \
		setsid $(BINDIR)/$(BIN) daemon >/dev/null 2>&1 < /dev/null & \
	fi

# deployed asks the DAEMON what build it is, rather than asking the binary that
# was just installed - those are different questions, and a stale daemon that
# survived the restart is the one way a deploy silently does nothing.
deployed: ## verify the running daemon is the build in $(BINDIR)
	@out=$$($(BINDIR)/$(BIN) status 2>&1) || { echo "$$out"; echo "DEPLOY FAILED: no daemon answered"; exit 1; }; \
	echo "$$out"; \
	case "$$out" in \
	  *"restart the daemon"*|*"does not report its version"*) \
	    echo "DEPLOY FAILED: the daemon serving is not the build in $(BINDIR)"; exit 1;; \
	esac; \
	echo "deployed to $(BINDIR)/$(BIN)"

rollback: ## restore the build deploy replaced and restart the daemon on it
	@test -x $(BINDIR)/$(BIN).prev || { echo "nothing to roll back to: no $(BINDIR)/$(BIN).prev"; exit 1; }
	@echo "rolling back to $$($(BINDIR)/$(BIN).prev version 2>/dev/null || echo 'an unreadable build')"
	-$(BINDIR)/$(BIN) quit 2>/dev/null || true
	@sleep 0.5
	-@$(kill-daemons)
	install -m 0755 $(BINDIR)/$(BIN).prev $(BINDIR)/$(BIN)
	@$(MAKE) --no-print-directory restart-daemon
	@sleep 1.5; $(MAKE) --no-print-directory deployed

install-bin: build ## install the binary to $(BINDIR) (no daemon restart)
	install -d $(BINDIR)
	install -m 0755 $(BIN) $(BINDIR)/$(BIN)

install-desktop: ## install the .desktop launcher and app icon (user, XDG)
	install -d $(DESKTOPDIR) $(ICONDIR)
	install -m 0644 packaging/agentbox.desktop $(DESKTOPDIR)/agentbox.desktop
	install -m 0644 internal/tray/icons/app-256.png $(ICONDIR)/agentbox.png
	-update-desktop-database $(DESKTOPDIR) 2>/dev/null || true
	-gtk-update-icon-cache -f -t $(HOME)/.local/share/icons/hicolor 2>/dev/null || true
	@echo "installed agentbox.desktop + icon"

install-service: ## install the systemd --user unit (enable it yourself)
	install -d $(UNITDIR)
	install -m 0644 packaging/agentbox.service $(UNITDIR)/agentbox.service
	-systemctl --user daemon-reload 2>/dev/null || true
	@echo "installed agentbox.service; start it with: systemctl --user enable --now agentbox.service"

install: install-bin install-desktop install-service ## binary + desktop launcher + systemd user service
	@echo "agentbox installed. Start it with: systemctl --user enable --now agentbox.service"

uninstall: ## remove binary, desktop entry, icon and service unit
	-systemctl --user disable --now agentbox.service 2>/dev/null || true
	rm -f $(BINDIR)/$(BIN) $(DESKTOPDIR)/agentbox.desktop $(ICONDIR)/agentbox.png $(UNITDIR)/agentbox.service
	-systemctl --user daemon-reload 2>/dev/null || true
	-update-desktop-database $(DESKTOPDIR) 2>/dev/null || true
	@echo "uninstalled agentbox"

# The deck is generated from tools/showcase/deck.py, which needs python-pptx. It
# lives in a throwaway venv rather than in the system python: one dependency for
# one script, and `rm -rf .venv-deck` undoes it.
DECKVENV = .venv-deck

# The gate is "can it import pptx", not "does bin/python exist". Those come apart:
# this machine had a venv with a python and no pip, so the old target skipped the
# venv build and then died on the pip line every time.
deck: ## rebuild docs/agentbox-showcase.pptx from tools/showcase/deck.py
	@$(DECKVENV)/bin/python -c 'import pptx' 2>/dev/null || { \
	    test -x $(DECKVENV)/bin/pip || python3 -m venv --clear $(DECKVENV); \
	    $(DECKVENV)/bin/pip install -q python-pptx; }
	@$(DECKVENV)/bin/python tools/showcase/deck.py

# ------------------------------------------------------------------------- wiki
#
# The public wiki is written in docs/wiki/pages and pushed to two repos that
# disagree about branch names, the landing page's filename and the sidebar's.
# Nothing on either platform mirrors a wiki repo, so this target is the only way
# a page reaches a reader. lint first: half the markdown GitLab renders is
# invisible or broken on GitHub, and it looks fine on the side you wrote it on.

wiki-check: ## lint the wiki source: portability, links, layout, prose tells
	@python3 tools/wiki/lint.py

wiki: ## publish docs/wiki/pages to the GitLab wiki and mirror it to GitHub
	@tools/wiki/publish.sh

wiki-dry: ## show what publishing would change, push nothing
	@tools/wiki/publish.sh --dry-run

clean: stop ## remove build output
	rm -f $(BIN)
