module github.com/openshift/source-to-image

go 1.13

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v0.2.10-0.20170808145631-06b9cb351610
	github.com/docker/docker => github.com/openshift/moby-moby v1.4.2-0.20190308215630-da810a85109d
	github.com/moby/buildkit => github.com/dmcgowan/buildkit v0.0.0-20170731200553-da2b9dc7dab9
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190708153700-3bdd9d9f5532
	google.golang.org/grpc => google.golang.org/grpc v1.3.0
)

exclude (
	github.com/coreos/bbolt v1.3.3
	github.com/coreos/etcd v3.3.15+incompatible
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20170629204627-19f72df4d05d // indirect
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/Microsoft/go-winio v0.4.12 // indirect
	github.com/Microsoft/hcsshim v0.6.3 // indirect
	github.com/Microsoft/opengcs v0.3.9 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/containerd/containerd v0.2.10-0.20170808145631-06b9cb351610 // indirect
	github.com/containerd/continuity v0.0.0-20190827140505-75bee3e2ccb6 // indirect
	github.com/coreos/bbolt v1.3.2 // indirect
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20190719114852-fd7a80b32e1f // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/deckarep/golang-set v1.7.1 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/docker/distribution v2.6.0-rc.1.0.20170726174610-edc3ab29cdff+incompatible
	github.com/docker/docker v0.0.0-20190404075923-dbe4a30928d4
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.3.2-0.20170127094116-9e638d38cf69 // indirect
	github.com/docker/libkv v0.2.1 // indirect
	github.com/docker/libnetwork v0.8.0-dev.2.0.20170816163629-5b28c0ec9823 // indirect
	github.com/docker/libtrust v0.0.0-20150526203908-9cbd2a1374f4 // indirect
	github.com/docker/swarmkit v1.12.1-0.20170818183140-ddb4539f883b // indirect
	github.com/fsnotify/fsnotify v1.4.7 // indirect
	github.com/go-check/check v0.0.0-20190902080502-41f04d3bba15 // indirect
	github.com/godbus/dbus v4.1.0+incompatible // indirect
	github.com/golang/groupcache v0.0.0-20190702054246-869f871628b6 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/gorilla/mux v1.7.3 // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.1 // indirect
	github.com/hashicorp/consul/api v1.2.0 // indirect
	github.com/hashicorp/go-memdb v1.0.3 // indirect
	github.com/hashicorp/memberlist v0.1.5 // indirect
	github.com/hashicorp/serf v0.8.3 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/mattn/go-shellwords v1.0.6 // indirect
	github.com/moby/buildkit v0.0.0-00010101000000-000000000000 // indirect
	github.com/mrunalp/fileutils v0.0.0-20171103030105-7d4729fb3618 // indirect
	github.com/opencontainers/go-digest v0.0.0-20170106003457-a6d0ee40d420 // indirect
	github.com/opencontainers/image-spec v1.0.0-rc6.0.20170604055404-372ad780f634 // indirect
	github.com/opencontainers/runc v1.0.0-rc4.0.20170825135527-4d6e6720a7c8 // indirect
	github.com/opencontainers/runtime-spec v1.0.1 // indirect
	github.com/opencontainers/selinux v1.3.0 // indirect
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/samuel/go-zookeeper v0.0.0-20190810000440-0ceca61e4d75 // indirect
	github.com/seccomp/libseccomp-golang v0.9.1 // indirect
	github.com/soheilhy/cmux v0.1.4 // indirect
	github.com/spf13/cobra v0.0.0-20160802223737-7c674d9e7201
	github.com/spf13/pflag v0.0.0-20170130214245-9ff6c6923cff
	github.com/stevvooe/continuity v0.0.0-20190827140505-75bee3e2ccb6 // indirect
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/tonistiigi/fifo v0.0.0-20190816180239-bda0ff6ed73c // indirect
	github.com/tonistiigi/fsutil v0.0.0-20170525050717-0ac4c11b053b // indirect
	github.com/vbatts/tar-split v0.11.1 // indirect
	github.com/vishvananda/netlink v0.0.0-20170924180554-177f1ceba557 // indirect
	github.com/vishvananda/netns v0.0.0-20190625233234-7109fa855b0f // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	go.etcd.io/bbolt v1.3.3 // indirect
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4 // indirect
	google.golang.org/grpc v1.23.1 // indirect
	gotest.tools v2.2.0+incompatible // indirect
	k8s.io/klog v0.4.0
)
