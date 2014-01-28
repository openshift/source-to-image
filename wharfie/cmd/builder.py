#!/usr/bin/env python

import sys
import docopt;

"""Wharfie is a tool for building reproducable Docker images.  Wharfie produces ready-to-run images by
injecting a user source into a docker image and /preparing/ a new Docker image which incorporates
the base image and built source, and is ready to use with `docker run`.  Wharfie supports
incremental builds which re-use previously downloaded dependencies, previously built artifacts, etc.
"""
class Builder(object):
    """
    Usage:
        wharfie build IMAGE_NAME SOURCE_DIR [--incremental=OLD_IMAGE_NAME] [--user=USERID]
        wharfie validate IMAGE_NAME [--supports-incremental]
        wharfie --help

    Arguments:
        IMAGE_NAME      Source image name. Wharfie will pull this image if not available locally.
        SOURCE_DIR      Directory containing your application sources.

    Options:
        --incremental=OLD_IMAGE_NAME    Perform an incremental build.
        --user=USERID                   Perform the build as specified user.
        --help                          Print this help message.
    """
    def __init__(self):
        self.arguments = docopt.docopt(Builder.__doc__)

    def validate_image(self, image_name, should_support_incremental):
        print("Validating image:%s incr-image:%s" % (image_name, should_support_incremental))

    def build(self, image_name, source_dir, incremental_image, user_id):
        print("Building image:%s dir:%s incr-image:%s uid:%s" % (image_name, source_dir, incremental_image, user_id))

    def main(self):
        if self.arguments['validate']:
            self.validate_image(self.arguments['IMAGE_NAME'], self.arguments['--supports-incremental']);

        if self.arguments['build']:
            if self.arguments['--incremental']:
                self.validate_image(self.arguments['IMAGE_NAME'], True);
                self.validate_image(self.arguments['--incremental'], True);
            else:
                self.validate_image(self.arguments['IMAGE_NAME'], False);

            self.build(self.arguments['IMAGE_NAME'], self.arguments['SOURCE_DIR'], self.arguments['--incremental'],
                       self.arguments['--user'])

def main():
    builder = Builder()
    builder.main()

if __name__ == "__main__":
    sys.path.insert(0, '.')
    main()
