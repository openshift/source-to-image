# Adding New Labels
New Docker Labels may be created and/or updated for the output image via the image metadata file.

If a new label is specified in the metadata file, the label will be added in the output image.  However, any label previously defined in the base builder image will be ***overwritten*** in the output image, if the same label name is specified in the image metadata file.

## Image Metadata File Name and Path
The name and path of the file ***must*** be the following:
```bash
/tmp/.s2i/image_metadata.json
```

## Example 
The file may have one or more label/value pairs.  Below is the JSON format of the labels, in the image metadata file:
```bash
{
  "labels": [
    {"labelkey1":"value1"},
    {"labelkey2":"value2"},
    .........
  ]
}

```
Note: If the JSON format is different than shown above, it will cause an error.

## Creating the File
The file should be created during the `assemble` step. 

## Notes on OpenShift 4.x
The feature of updating output image labels as described above is currently not working in OpenShift 4.x at the time of writing this section (04.14.2020). See the error report at https://bugzilla.redhat.com/show_bug.cgi?id=1758305#c20

A workaround is to update the output image labels during building by adding custom labels to the BuildConfig as described in https://docs.openshift.com/container-platform/4.3/builds/managing-build-output.html#builds-output-image-labels_managing-build-output

