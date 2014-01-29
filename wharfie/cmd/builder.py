#!/usr/bin/env python

import sys
import docopt;
import docker;
import time;

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
        self.docker_client = docker.Client("http://10.211.55.27:5555")
        server_version = self.docker_client.version()
        print "Connected to Docker server version %s. Server linux kernel: %s" % \
              (server_version['Version'],server_version['KernelVersion'])

    def check_file_exists(self, container_id, file_path):
        try:
            self.docker_client.copy(container_id, file_path)
            return True
        except docker.APIError as e:
            return False

    def validate_image(self, image_name, should_support_incremental):
        images = self.docker_client.images(image_name)
        if images.__len__() == 0:
            print("Pulling image %s" % image_name)
            self.docker_client.pull(image_name)
        else:
            print("Image %s is available in local registry" % image_name)

        image = self.docker_client.inspect_image(images[0]['Id'])
        if image['config']['Entrypoint']:
            print("Image %s has a configured Entrypoint and is incompatible with wharfi" % image_name)
            return False

        valid_image = True
        required_files = ['/usr/bin/prepare', '/usr/bin/run']
        if should_support_incremental:
            required_files += ['/usr/bin/save-artifacts']
        try:
            container = self.docker_client.create_container(image_name)
            container_id = container['Id']
            for f in required_files:
                if not self.check_file_exists(container_id, f):
                    valid_image = False
                    print("Invalid image: file %s is missing." % f)
            self.docker_client.stop(container)
            self.docker_client.remove_container(container)
            return valid_image
        except docker.APIError as e:
            print("Error while creating container for image %s. %s" % (image_name, e.explanation))
            return False

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

        self.docker_client.close()

def main():
    builder = Builder()
    builder.main()

if __name__ == "__main__":
    sys.path.insert(0, '.')
    main()
