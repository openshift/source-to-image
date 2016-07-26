<!-- BEGIN MUNGE: UNVERSIONED_WARNING -->

<!-- BEGIN STRIP_FOR_RELEASE -->

<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">

<h2>PLEASE NOTE: This document applies to the HEAD of the source tree</h2>

If you are using a released version of Kubernetes, you should
refer to the docs that go with that version.

<!-- TAG RELEASE_LINK, added by the munger automatically -->
<strong>
The latest release of this document can be found
[here](http://releases.k8s.io/release-1.2/examples/flexvolume/README.md).

Documentation for other releases can be found at
[releases.k8s.io](http://releases.k8s.io).
</strong>
--

<!-- END STRIP_FOR_RELEASE -->

<!-- END MUNGE: UNVERSIONED_WARNING -->

# Flexvolume

Flexvolume enables users to mount vendor volumes into kubernetes. It expects vendor drivers are installed in the volume plugin path on every kubelet node.

It allows for vendors to develop their own drivers to mount volumes on nodes.

*Note: Flexvolume is an alpha feature and is most likely to change in future*

## Prerequisites

Install the vendor driver on all nodes in the kubelet plugin path. Path for installing the plugin: /usr/libexec/kubernetes/kubelet-plugins/volume/exec/\<vendor~driver\>/\<driver\>

For example to add a 'cifs' driver, by vendor 'foo' install the driver at: /usr/libexec/kubernetes/kubelet-plugins/volume/exec/\<foo~cifs\>/cifs

## Plugin details

Driver will be invoked with 'Init' to initialize the driver. It will be invoked with 'attach' to attach the volume and with 'detach' to detach the volume from the kubelet node. It also supports custom mounts using 'mount' and 'unmount' callouts to the driver.

### Driver invocation model:

Init:

```
<driver executable> init
```

Attach:

```
<driver executable> attach <json options>
```

Detach:

```
<driver executable> detach <mount device>
```

Mount:

```
<driver executable> mount <target mount dir> <mount device> <json options>
```

Unmount:

```
<driver executable> unmount <mount dir>
```

See lvm[lvm] for a quick example on how to write a simple flexvolume driver.

### Driver output:

Flexvolume expects the driver to reply with the status of the operation in the
following format.

```
{
	"status": "<Success/Failure>",
	"message": "<Reason for success/failure>",
	"device": "<Path to the device attached. This field is valid only for attach calls>"
}
```

### Default Json options

In addition to the flags specified by the user in the Options field of the FlexVolumeSource, the following flags are also passed to the executable.

```
"kubernetes.io/fsType":"<FS type>",
"kubernetes.io/readwrite":"<rw>",
"kubernetes.io/secret/key1":"<secret1>"
...
"kubernetes.io/secret/keyN":"<secretN>"
```

### Example of Flexvolume

See nginx.yaml[nginx.yaml] for a quick example on how to use Flexvolume in a pod.

<!-- BEGIN MUNGE: GENERATED_ANALYTICS -->
[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/examples/flexvolume/README.md?pixel)]()
<!-- END MUNGE: GENERATED_ANALYTICS -->
