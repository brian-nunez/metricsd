PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
UNITDIR ?= $(HOME)/.config/systemd/user
GO      ?= go

GOFLAGS := -trimpath
LDFLAGS := -s -w

.PHONY: build vet install uninstall clean

build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/ ./cmd/...

vet:
	$(GO) vet ./...

install: build
	install -Dm755 bin/metricsd $(BINDIR)/metricsd
	install -Dm755 bin/metricsctl $(BINDIR)/metricsctl
	install -d $(UNITDIR)
	sed 's|@BINDIR@|$(BINDIR)|g' deploy/metricsd.service > $(UNITDIR)/metricsd.service
	chmod 644 $(UNITDIR)/metricsd.service
	systemctl --user daemon-reload
	@echo "Installed. Start with: systemctl --user enable --now metricsd"

uninstall:
	-systemctl --user disable --now metricsd
	rm -f $(BINDIR)/metricsd $(BINDIR)/metricsctl $(UNITDIR)/metricsd.service
	systemctl --user daemon-reload
	@echo "Data left in place: $${XDG_STATE_HOME:-$(HOME)/.local/state}/metricsd"

clean:
	rm -rf bin
