PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
UNITDIR ?= $(HOME)/.config/systemd/user
GO      ?= go

# Release builds pass VERSION (e.g. v1.2.3); other builds report the
# module or VCS version from Go's build info.
VERSION ?=

GOFLAGS := -trimpath
LDFLAGS := -s -w $(if $(VERSION),-X github.com/brian-nunez/metricsd/internal/version.version=$(VERSION))

# Release archive targets, named as they appear in asset file names.
DIST_ARCHES := amd64 arm64 armv7

.PHONY: build vet install uninstall dist clean

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/ . ./metricsctl

vet:
	$(GO) vet ./...

install: build
	install -Dm755 bin/metricsd $(BINDIR)/metricsd
	install -Dm755 bin/metricsctl $(BINDIR)/metricsctl
	install -d $(UNITDIR)
	sed 's|@BINDIR@|$(BINDIR)|g' deploy/metricsd.service > $(UNITDIR)/metricsd.service
	chmod 644 $(UNITDIR)/metricsd.service
	systemctl --user daemon-reload
	@# A running service keeps the old binary in memory until it restarts.
	@if systemctl --user is-active --quiet metricsd; then \
		systemctl --user try-restart metricsd && echo "Installed and restarted metricsd."; \
	else \
		echo "Installed. Start with: systemctl --user enable --now metricsd"; \
	fi

uninstall:
	-systemctl --user disable --now metricsd
	rm -f $(BINDIR)/metricsd $(BINDIR)/metricsctl $(UNITDIR)/metricsd.service
	systemctl --user daemon-reload
	@echo "Data left in place: $${XDG_STATE_HOME:-$(HOME)/.local/state}/metricsd"

# Asset names carry no version so the latest release is always reachable at
# .../releases/latest/download/metricsd_linux_<arch>.tar.gz.
dist:
	@test -n "$(VERSION)" || { echo "usage: make dist VERSION=vX.Y.Z" >&2; exit 1; }
	rm -rf dist
	@set -e; for arch in $(DIST_ARCHES); do \
		name=metricsd_linux_$$arch; \
		goarch=$$arch; goarm=; \
		if [ $$arch = armv7 ]; then goarch=arm; goarm=7; fi; \
		echo "build $$name"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$goarch GOARM=$$goarm \
			$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o dist/$$name/ . ./metricsctl; \
		cp LICENSE README.md deploy/metricsd.service deploy/config.example.toml dist/$$name/; \
		tar -C dist -czf dist/$$name.tar.gz $$name; \
		rm -rf dist/$$name; \
	done
	cd dist && sha256sum *.tar.gz > checksums.txt

clean:
	rm -rf bin dist
