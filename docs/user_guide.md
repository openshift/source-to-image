# Using S2I images

S2I builder images normally include `assemble` and `run` scripts, but the default behavior of those scripts may not be suitable for all users. This document will cover a few approaches for customizing the behavior of an S2I builder that includes default scripts.

## Invoking scripts embedded in an image

Typically builder images provide their own version of the [S2I scripts](builder_image.md#s2i-scripts) that cover the most common use-cases. If these scripts don't fulfill your needs, S2I provides a way of overriding them by adding custom ones in the `.s2i/bin` directory. However, by doing this you are completely replacing the standard scripts. In some cases this is acceptable but in other scenarios you may prefer to execute a few commands before (or after) the scripts while retaining the logic of the script provided in the image. In this case it is possible to create a wrapper script that will execute custom logic and delegate further work to the default script in the image.

To determine the location of the scripts inside of the builder image, we need to look at at the value of `io.openshift.s2i.scripts-url` label. We can use `docker inspect` for that:

```console
$ docker inspect --format='{{ index .Config.Labels "io.openshift.s2i.scripts-url" }}' openshift/wildfly-100-centos7
image:///usr/libexec/s2i
```

Here we are inspected the `openshift/wildfly-100-centos7` builder image and found out that the scripts are in the `/usr/libexec/s2i` directory.

With this knowledge we can invoke any of these scripts from our own by wrapping its invocation.

Example of `.s2i/bin/assemble` script:
```bash
#!/bin/bash
echo "Before assembling"

/usr/libexec/s2i/assemble
rc=$?

if [ $rc -eq 0 ]; then
    echo "After successful assembling"
else
    echo "After failed assembling"
fi

exit $rc
```

The example shows a custom `assemble` script that prints the message, executes standard `assemble` script from the image and prints another message depending on the exit code of the `assemble` script.

Wrapping the `run` script is a bit tricky because we have to use `exec` for invoking it (see chapter ["Always `exec` in Wrapper Scripts" in the Guidance for Docker Image Authors](http://www.projectatomic.io/docs/docker-image-author-guidance/)). As a consequence, it is impossible to have a post-run step because the code after `exec /usr/libexec/s2i/run` never gets executed.

Example of `.s2i/bin/run` script:
```bash
#!/bin/bash
echo "Before running application"
exec /usr/libexec/s2i/run
```
