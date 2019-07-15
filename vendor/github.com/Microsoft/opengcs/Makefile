BASE:=base.tar.gz

GO:=go
GO_FLAGS:=-ldflags "-s -w" # strip Go binaries
CGO_ENABLED:=0
GOMODVENDOR:=

CFLAGS:=-O2 -Wall
LDFLAGS:=-static -s # strip C binaries

GO_FLAGS_EXTRA:=
ifeq "$(GOMODVENDOR)" "1"
GO_FLAGS_EXTRA += -mod=vendor
endif
GO_BUILD:=CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GO_FLAGS) $(GO_FLAGS_EXTRA)

SRCROOT=$(dir $(abspath $(firstword $(MAKEFILE_LIST))))

# The link aliases for gcstools
GCS_TOOLS=\
	vhd2tar \
	exportSandbox \
	netnscfg \
	remotefs

.PHONY: all always rootfs test

all: out/initrd.img out/rootfs.tar.gz

clean:
	find -name '*.o' -print0 | xargs -0 -r rm
	rm -rf bin deps rootfs out

test:
	cd $(SRCROOT) && go test ./service/gcsutils/...
	cd $(SRCROOT)/service/gcs && ginkgo -r -keepGoing

out/delta.tar.gz: bin/init bin/vsockexec bin/service/gcs bin/service/gcsutils/gcstools Makefile
	@mkdir -p out
	rm -rf rootfs
	mkdir -p rootfs/bin/
	cp bin/init rootfs/
	cp bin/vsockexec rootfs/bin/
	cp bin/service/gcs rootfs/bin/
	cp bin/service/gcsutils/gcstools rootfs/bin/
	for tool in $(GCS_TOOLS); do ln -s gcstools rootfs/bin/$$tool; done
	git -C $(SRCROOT) rev-parse HEAD > rootfs/gcs.commit && \
	git -C $(SRCROOT) rev-parse --abbrev-ref HEAD > rootfs/gcs.branch
	tar -zcf $@ -C rootfs .
	rm -rf rootfs

out/rootfs.tar.gz: out/initrd.img
	rm -rf rootfs-conv
	mkdir rootfs-conv
	gunzip -c out/initrd.img | (cd rootfs-conv && cpio -imd)
	tar -zcf $@ -C rootfs-conv .
	rm -rf rootfs-conv

out/initrd.img: $(BASE) out/delta.tar.gz $(SRCROOT)/hack/catcpio.sh
	$(SRCROOT)/hack/catcpio.sh "$(BASE)" out/delta.tar.gz > out/initrd.img.uncompressed
	gzip -c out/initrd.img.uncompressed > $@
	rm out/initrd.img.uncompressed

-include deps/service/gcs.gomake
-include deps/service/gcsutils/gcstools.gomake

# Implicit rule for includes that define Go targets.
%.gomake: $(SRCROOT)/Makefile
	@mkdir -p $(dir $@)
	@/bin/echo $(@:deps/%.gomake=bin/%): $(SRCROOT)/hack/gomakedeps.sh > $@.new
	@/bin/echo -e '\t@mkdir -p $$(dir $$@) $(dir $@)' >> $@.new
	@/bin/echo -e '\t$$(GO_BUILD) -o $$@.new $$(SRCROOT)/$$(@:bin/%=%)' >> $@.new
	@/bin/echo -e '\tGO="$(GO)" $$(SRCROOT)/hack/gomakedeps.sh $$@ $$(SRCROOT)/$$(@:bin/%=%) $$(GO_FLAGS) $$(GO_FLAGS_EXTRA) > $(@:%.gomake=%.godeps).new' >> $@.new
	@/bin/echo -e '\tmv $(@:%.gomake=%.godeps).new $(@:%.gomake=%.godeps)' >> $@.new
	@/bin/echo -e '\tmv $$@.new $$@' >> $@.new
	@/bin/echo -e '-include $(@:%.gomake=%.godeps)' >> $@.new
	mv $@.new $@

VPATH=$(SRCROOT)

bin/vsockexec: vsockexec/vsockexec.o
	@mkdir -p bin
	$(CC) $(LDFLAGS) -o $@ $<

bin/init: init/init.o
	@mkdir -p bin
	$(CC) $(LDFLAGS) -o $@ $<

%.o: %.c
	@mkdir -p $(dir $@)
	$(CC) $(CFLAGS) $(CPPFLAGS) -c -o $@ $<
