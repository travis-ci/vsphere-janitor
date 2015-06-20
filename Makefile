PACKAGE := github.com/travis-ci/vmware-janitor
VERSION_VAR := $(PACKAGE).VersionString
VERSION_VALUE ?= $(shell git describe --always --dirty --tags 2>/dev/null)
REV_VAR := $(PACKAGE).RevisionString
REV_VALUE ?= $(shell git rev-parse --sq HEAD 2>/dev/null || echo "'???'")
GENERATED_VAR := $(PACKAGE).GeneratedString
GENERATED_VALUE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%S%z')

FIND ?= find
XARGS ?= xargs
GB ?= gb

GOBUILD_LDFLAGS ?= -ldflags "\
	-X $(VERSION_VAR) '$(VERSION_VALUE)' \
	-X $(REV_VAR) $(REV_VALUE) \
	-X $(GENERATED_VAR) '$(GENERATED_VALUE)' \
"

.PHONY: all
all: clean test

.PHONY: test
test: build .test

.PHONY: .test
.test:
	$(GB) test

.PHONY: clean
clean:
	$(FIND) pkg -name '*.a' | $(XARGS) rm -vf

.PHONY: build
build:
	$(GB) build $(GOBUILD_LDFLAGS)

.PHONY: update
update:
	$(GB) vendor update --all
