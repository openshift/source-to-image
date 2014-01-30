#!/usr/bin/env python

from colorama import *
import os
import sys
import docopt
import docker
import tempfile
import shutil

"""Wharfie is a tool for building reproducable Docker images.  Wharfie produces ready-to-run images by
injecting a user source into a docker image and preparing a new Docker image which incorporates
the base image and built source, and is ready to use with `docker run`.  Wharfie supports
incremental builds which re-use previously downloaded dependencies, previously built artifacts, etc.
"""
class Builder(object):
    """
    Wharfie.

    Usage:
        wharfie build IMAGE_NAME SOURCE_DIR [--tag=BUILD_TAG] [--incremental=PREV_BUILD]
            [--user=USERID] [--url=URL] [--timeout=TIMEOUT] [-e ENV_NAME=VALUE]...
        wharfie validate IMAGE_NAME [--supports-incremental] [--url=URL] [--timeout=TIMEOUT]
        wharfie --help

    Arguments:
        IMAGE_NAME      Source image name. Wharfie will pull this image if not available locally.
        SOURCE_DIR      Directory containing your application sources.

    Options:
        --incremental=PREV_BUILD        Perform an incremental build. PREV_BUILD specified the previous built image.
        --tag=BUILD_TAG                 Image will be tagged with provided name after a successful build.
        --timeout=TIMEOUT               Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]
        --help                          Print this help message.
    """
    def __init__(self):
        self.arguments = docopt.docopt(Builder.__doc__)
        self.docker_url = self.arguments['--url']

        if self.arguments['--timeout']:
            self.timeout = float(self.arguments['--timeout'])
        else:
            self.timeout = 120

        self.docker_client = docker.Client(base_url=self.docker_url, timeout=self.timeout)
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

    def build(self, image_name, source_dir, incremental_image, user_id, tag, envs=[]):
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

            build_context_source = os.path.join(tmp_dir, 'src')
            shutil.copytree(source_dir, build_context_source)

            docker_file = open(os.path.join(tmp_dir, 'Dockerfile'), 'w+')
            docker_file.write("FROM %s\n" % image_name)
            docker_file.write('ADD ./src /usr/src/\n')
            if incremental_image:
                docker_file.write('ADD ./artifacts /usr/artifacts\n')
            for env in envs:
                env = env.split("=")
                name = env[0]
                value = env[1]
                docker_file.write("ENV %s %s\n" % (name, value))
            docker_file.write('RUN /usr/bin/prepare\n')
            docker_file.write('CMD /usr/bin/run\n')
            docker_file.close()
            
            print("Building new docker image")
            img, logs = self.docker_client.build(tag=tag, path=tmp_dir, rm=True)
            print("Build logs: %s" % logs)

            if img is not None:
                built_image_name = tag or img

                print(Fore.GREEN + "Built image %s" % built_image_name)
            else:
                print(Fore.RED + "Wharfie build failed.")

        finally:
            shutil.rmtree(tmp_dir)

    def main(self):
        image_name = self.arguments['IMAGE_NAME']
        try:
            if self.arguments['validate']:
                self.validate_image(image_name, self.arguments['--supports-incremental']);

            if self.arguments['build']:
                tag = self.arguments['--tag']

                if self.arguments['--incremental']:
                    if ((not self.validate_image(image_name, True)) or
                            (not self.validate_image(self.arguments['--incremental'], True))):
                        return -1
                else:
                    if not self.validate_image(image_name, False):
                        return -1

                self.build(image_name, self.arguments['SOURCE_DIR'], self.arguments['--incremental'],
                           self.arguments['--user'], tag, self.arguments['ENV_NAME=VALUE'])
        finally:
            self.docker_client.close()


def main():
    builder = Builder()
    builder.main()

if __name__ == "__main__":
    sys.path.insert(0, '.')
    main()
