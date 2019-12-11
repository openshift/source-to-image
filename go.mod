module github.com/openshift/source-to-image

go 1.13

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v0.2.10-0.20170808145631-06b9cb351610
	github.com/docker/docker => github.com/openshift/moby-moby v1.4.2-0.20190308215630-da810a85109d
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190708153700-3bdd9d9f5532
	google.golang.org/grpc => google.golang.org/grpc v1.3.0
)

exclude (
	github.com/coreos/bbolt v1.3.3
	github.com/coreos/etcd v3.3.15+incompatible
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20170629204627-19f72df4d05d // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/docker v1.14.0-0.20190319215453-e7b5f7dbe98c
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.3.2-0.20170127094116-9e638d38cf69 // indirect
	github.com/docker/libtrust v0.0.0-20150526203908-9cbd2a1374f4 // indirect
	github.com/gogo/protobuf v1.2.1 // indirect
	github.com/google/go-cmp v0.3.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/moby/buildkit v0.6.3
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/spf13/cobra v0.0.0-20160802223737-7c674d9e7201
	github.com/spf13/pflag v0.0.0-20170130214245-9ff6c6923cff
	github.com/stretchr/testify v1.4.0 // indirect
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	golang.org/x/sys v0.0.0-20190801041406-cbf593c0f2f3 // indirect
	k8s.io/klog v0.4.0
)
