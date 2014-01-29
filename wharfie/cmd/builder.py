#!/usr/bin/env python

import os
import sys
import docopt
import docker
import io
import tempfile
import shutil
from array import array

"""Wharfie is a tool for building reproducable Docker images.  Wharfie produces ready-to-run images by
injecting a user source into a docker image and preparing a new Docker image which incorporates
the base image and built source, and is ready to use with `docker run`.  Wharfie supports
incremental builds which re-use previously downloaded dependencies, previously built artifacts, etc.
"""
class Builder(object):
    """
    Wharfie.

    Usage:
        wharfie build NEW_IMAGE_NAME IMAGE_NAME SOURCE_DIR [--incremental=OLD_IMAGE_NAME] [--user=USERID] [--url=URL]
        wharfie validate IMAGE_NAME [--supports-incremental] [--url=URL]
        wharfie --help

    Arguments:
        IMAGE_NAME      Source image name. Wharfie will pull this image if not available locally.
        SOURCE_DIR      Directory containing your application sources.

    Options:
        --incremental=OLD_IMAGE_NAME    Perform an incremental build.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]
        --help                          Print this help message.
    """
    def __init__(self):
        self.arguments = docopt.docopt(Builder.__doc__)
        self.docker_url = self.arguments['--url']
        # TODO: make timeout an option or smarter
        self.docker_client = docker.Client(base_url=self.docker_url, timeout=1000)
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
            print("Image %s has a configured Entrypoint and is incompatible with wharfie" % image_name)
            return False

        valid_image = True
        required_files = ['/usr/bin/prepare', '/usr/bin/run']
        if should_support_incremental:
            required_files += ['/usr/bin/save-artifacts']
        try:
            container = self.docker_client.create_container(image_name, command='true')
            container_id = container['Id']
            for f in required_files:
                if not self.check_file_exists(container_id, f):
                    valid_image = False
                    print("Invalid image: file %s is missing." % f)
            self.docker_client.stop(container)
            self.docker_client.remove_container(container)

            if valid_image:
                print("%s passes source image validation" % image_name)

            return valid_image
        except docker.APIError as e:
            print("Error while creating container for image %s. %s" % (image_name, e.explanation))
            return False

    def build(self, new_image_name, image_name, source_dir, incremental_image, user_id):
        tmp_dir = tempfile.mkdtemp()

        try:
            if incremental_image:
                artifact_tmp_dir = os.path.join(tmp_dir, 'artifacts')
                container = self.docker_client.create_container(image_name,
                                                                ["/usr/bin/save-artifacts"],
                                                                volumes={"/usr/artifacts": {}})
                container_id = container['Id']
                self.docker_client.start(container_id, binds={artifact_tmp_dir: "/usr/artifacts"})
                exitcode = self.client.wait(container_id)
                self.docker_client.remove_container(container)

            dockerfile_lines = ["FROM %s" % image_name]

            build_context_source = os.path.join(tmp_dir, 'src')
            shutil.copytree(source_dir, build_context_source)
            dockerfile_lines.append("ADD ./src /usr/src/")

            if incremental_image:
                dockerfile_lines.append("ADD ./artifacts /usr/artifacts")

            dockerfile_lines.append('RUN /usr/bin/prepare')
            dockerfile_lines.append('CMD /usr/bin/run')
            
            dockerfile = open(os.path.join(tmp_dir, 'Dockerfile'), 'w+')
            for line in dockerfile_lines:
                print>>dockerfile, line
            dockerfile.close()

            print("Building new docker image")
            img, logs = self.docker_client.build(tag=new_image_name, path=tmp_dir)
            print("Build logs: %s" % logs)

            print("Built image %s" % new_image_name)
        finally:
            shutil.rmtree(tmp_dir)

    def main(self):
        image_name = self.arguments['IMAGE_NAME']

        if self.arguments['validate']:
            self.validate_image(image_name, self.arguments['--supports-incremental']);

        if self.arguments['build']:
            new_image_name = self.arguments['NEW_IMAGE_NAME']

            if self.arguments['--incremental']:
                self.validate_image(image_name, True);
                self.validate_image(self.arguments['--incremental'], True);
            else:
                self.validate_image(image_name, False);

            self.build(new_image_name, image_name, self.arguments['SOURCE_DIR'], self.arguments['--incremental'],
                       self.arguments['--user'])

        self.docker_client.close()


def main():
    builder = Builder()
    builder.main()

if __name__ == "__main__":
    sys.path.insert(0, '.')
    main()
