# How to construct user-mode components

Under the `/` directory, the following subdirectories are required:

- `/tmp`
- `/proc`
- `/bin`
- `/dev`
- `/run`
- `/etc`
- `/usr`
- `/mnt`
- `/sys`

- `/init`
- `/root`
- `/sbin`
- `/lib64`
- `/lib`

Here are the expected contents of each subdirectory /file

1. Subdirectories with **empty** contents:  `/tmp` `/proc` `/dev` `/run` `/etc` `/usr` `/mnt` `/sys`

2. **/init**
   This is the [init script file](../kernel/scripts/init_script)

3. **/root** : this is the home directory of the root account.

4. **/sbin** :
    - /sbin/runc

              Note:this is the "runc" binary for hosting the container execution environment.

              `runc -v` (where `runc` was installed using `go get`, `go install`, or `go build`) should output the following:
                  runc version spec: 1.0.0

              `runc -v` (where `runc` was installed using the Makefile in the runc repo) should output the following:
                  runc version 1.0.0-rc4+dev
                  commit: 3f2f8b84a77f73d38244dd690525642a72156c64
                  spec: 1.0.0

    - /sbin/[udhcpc_config.script](https://github.com/mirror/busybox/blob/master/examples/udhcp/simple.script)

5. **/lib64** :

       /lib64/ld-linux-x86-64.so.2

6. **/lib** :

       /lib/x86_64-linux-gnu
       /lib/x86_64-linux-gnu/libe2p.so.2
       /lib/x86_64-linux-gnu/libcom_err.so.2
       /lib/x86_64-linux-gnu/libc.so.6
       /lib/x86_64-linux-gnu/libdl.so.2
       /lib/x86_64-linux-gnu/libapparmor.so.1
       /lib/x86_64-linux-gnu/libseccomp.so.2
       /lib/x86_64-linux-gnu/libblkid.so.1
       /lib/x86_64-linux-gnu/libpthread.so.0
       /lib/x86_64-linux-gnu/libext2fs.so.2
       /lib/x86_64-linux-gnu/libuuid.so.1
       /lib/modules

7. **/bin** : binaries in this subdir are categorised into four groups

    - [GCS binaries](gcsbuildinstructions.md)

            /bin/exportSandbox
            /bin/gcs
            /bin/gcstools
            /bin/netnscfg
            /bin/remotefs
            /bin/vhd2tar

            Note : exportSandbox, vhd2tar, remotefs, and netnscfg are actually hard links to the "gcstools' file

    - Required binaires: utilities used by gcs

             /bin/hostname
             /bin/mkdir
             /bin/mount
             /bin/rmdir
             /bin/sh

             /sbin/blockdev
             /sbin/
             /sbin/ip
             /sbin/iproute
             /sbin/mkfs.ext4
             /sbin/udhcp

    - Required binaires: utilities used by docker

             /bin/cat
             /bin/kill
             /bin/ls
             /bin/pidof

    - Debugging tools: mostly from busybox tool set

7. **/** :

    - /gcs.commit Optional file containing the commit id of Microsoft/opengcs
