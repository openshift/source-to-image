#!/usr/bin/env python

from colorama import *
import os
import sys
import docopt
import docker
import tempfile
import shutil
import time
import logging

"""STI is a tool for building reproducable Docker images.  STI produces ready-to-run images by
injecting a user source into a docker image and preparing a new Docker image which incorporates
the base image and built source, and is ready to use with `docker run`.  STI supports
incremental builds which re-use previously downloaded dependencies, previously built artifacts, etc.
"""
class Builder(object):
    """
    Docker Source to Image.

    Usage:
        sti build IMAGE_NAME SOURCE_DIR --tag=BUILD_TAG [--build-image=BUILD_IMAGE_NAME] [--clean]
            [--user=USERID] [--url=URL] [--timeout=TIMEOUT] [-e ENV_NAME=VALUE]... [-l LOG_LEVEL]
        sti validate IMAGE_NAME [--supports-incremental] [--url=URL] [--timeout=TIMEOUT] [-l LOG_LEVEL]
        sti --help

    Arguments:
        IMAGE_NAME      Source image name. STI will pull this image if not available locally.
        SOURCE_DIR      Directory containing your application sources.

    Options:
        --clean                         Do a clean build, ie. do not re-use the context from an earlier build
        -l LOG_LEVEL                    Logging level. Default: INFO
        --tag=BUILD_TAG                 Image will be tagged with provided name after a successful build.
        --timeout=TIMEOUT               Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]
        --help                          Print this help message.
    """
    def __init__(self):
        self.arguments = docopt.docopt(Builder.__doc__)
        log_level = self.arguments['-l'] or "INFO"
        numeric_level = getattr(logging, log_level.upper(), None)
        if not isinstance(numeric_level, int):
            logging.warn("Invalid log level %s. Defaulting to INFO", log_level)
            numeric_level = logging.INFO;
        logging.basicConfig(level=numeric_level)
        self.logger = logging.getLogger(__name__)

        self.docker_url = self.arguments['--url']

        if self.arguments['--timeout']:
            self.timeout = float(self.arguments['--timeout'])
        else:
            self.timeout = 120

        self.docker_client = docker.Client(base_url=self.docker_url, timeout=self.timeout)
        server_version = self.docker_client.version()
        self.logger.debug("Connected to Docker server version %s. Server linux kernel: %s",
              server_version['Version'],server_version['KernelVersion'])

    def check_file_exists(self, container_id, file_path):
        try:
            self.docker_client.copy(container_id, file_path)
            return True
        except docker.APIError as e:
            return False

    def pull_image(self, image_name):
        images = self.docker_client.images(image_name)
        if images.__len__() == 0:
            self.logger.warn("Image %s not found in local registry. Pulling from remote." , image_name)
            self.docker_client.pull(image_name)
        else:
            self.logger.debug("Image %s is available in local registry" % image_name)

    def container_from_image(self, image_name):
        try:
            container = self.docker_client.create_container(image_name, command='/bin/true')
            container_id = container['Id']
            self.docker_client.start(container_id)
            exitcode = self.docker_client.wait(container_id)
            time.sleep(1)

            return container_id
        except docker.APIError as e:
            self.logger.critical("Error while creating container for image %s. %s" , image_name, e.explanation)
            return None

    def remove_container(self, container_id):
        self.docker_client.remove_container(container_id)

    def validate_images(self, requests=[]):
        for request in requests:
            image_name = request.image_name
            self.pull_image(image_name)
            container_id = self.container_from_image(image_name)

            if not container_id:
                return False

            valid = self.validate_image(image_name, container_id, request.validate_incremental)
            self.remove_container(container_id)

            if not valid:
                self.logger.critical("%s %s failed validation %s" % (Fore.RED, request.description, Fore.RESET))
                return False
        return True

    def validate_image(self, image_name, container_id, validate_incremental):
        images = self.docker_client.images(image_name)

        if len(images) < 1:
            self.logger.critical("Couldn't find image %s" % image_name)
            return False

        image = self.docker_client.inspect_image(images[0])

        if image['config']['Entrypoint']:
            self.logger.critical("Image %s has a configured Entrypoint and is incompatible with sti" , image_name)
            return False

        required_files = ['/usr/bin/prepare', '/usr/bin/run']
        if validate_incremental:
            required_files += ['/usr/bin/save-artifacts']

        valid_image = self.validate_container(container_id, required_files)

        if valid_image:
            self.logger.debug("%s passes source image validation" , image_name)

        return valid_image

    def validate_container(self, container_id, required_files=[]):
        valid_image = True

        for f in required_files:
            if not self.check_file_exists(container_id, f):
                valid_image = False
                self.logger.critical("Invalid image: file %s is missing." , f)

        return valid_image

    def detect_incremental_build(self, image_name):
        container_id = self.container_from_image(image_name)

        try:
            result = self.check_file_exists(container_id, '/usr/bin/save-artifacts')
            self.remove_container(container_id)

            return result
        except docker.APIError as e:
            self.logger.critical("Error while detecting whether image %s supports incremental build" % image_name)
            return False

    def direct_build(self, image_name, source_dir, incremental_image, user_id, tag, envs=[]):
        tmp_dir = tempfile.mkdtemp()

        try:
            if incremental_image:
                self.logger.debug("Saving data from %s for incremental build" , incremental_image)
                artifact_tmp_dir = os.path.join(tmp_dir, 'artifacts')
                os.mkdir(artifact_tmp_dir)
                container = self.docker_client.create_container(incremental_image,
                                                                ["/usr/bin/save-artifacts"],
                                                                volumes={"/usr/artifacts": {}})
                container_id = container['Id']
                self.docker_client.start(container_id, binds={artifact_tmp_dir: "/usr/artifacts"})
                exitcode = self.docker_client.wait(container_id)
                self.logger.debug(self.docker_client.logs(container_id))
                time.sleep(1)
                self.docker_client.remove_container(container_id)
            build_context_source = os.path.join(tmp_dir, 'src')

            if re.match('^(http(s?)|git|file)://', source_dir):
                git_clone_cmd = "git clone --quiet %s %s" %(source_dir, build_context_source)
                try:
                    self.logger.debug("Fetching %s", source_dir)
                    subprocess.check_output(git_clone_cmd, stderr=subprocess.STDOUT, shell=True)
                except subprocess.CalledProcessError as e:
                    self.logger.critical("%s command failed (%i)", git_clone_cmd, e.returncode)
                    return False
            else:
                shutil.copytree(source_dir, build_context_source)

            with open(os.path.join(tmp_dir, 'Dockerfile'), 'w+') as docker_file:
                docker_file.write("FROM %s\n" % image_name)
                docker_file.write('ADD ./src /usr/src/\n')
                if incremental_image:
                    docker_file.write('ADD ./artifacts /usr/artifacts/\n')
                for env in envs:
                    env = env.split("=")
                    name = env[0]
                    value = env[1]
                    docker_file.write("ENV %s %s\n" % (name, value))
                docker_file.write('RUN /usr/bin/prepare\n')
                docker_file.write('CMD /usr/bin/run\n')

            self.logger.debug("Building new docker image")
            img, logs = self.docker_client.build(tag=tag, path=tmp_dir, rm=True)
            self.logger.debug("Build logs: %s" , logs)

            if img is not None:
                built_image_name = tag or img
                self.logger.info("%s Built image %s %s" , Fore.GREEN, built_image_name, Fore.RESET)
            else:
                self.logger.critical("%s STI build failed. %s", Fore.RED, Fore.RESET)

        finally:
            shutil.rmtree(tmp_dir)
            pass

    def main(self):
        runtime_image_name = self.arguments['IMAGE_NAME']
        validations = []
        try:
            if self.arguments['validate']:
                validations.append(ImageValidationRequest('Target image', runtime_image_name, self.arguments['--supports-incremental']))
            elif self.arguments['build']:
                validations.append(ImageValidationRequest('Runtime image', runtime_image_name))
                build_image_name = self.arguments['--build-image']
                validations.append(ImageValidationRequest('Build image', runtime_image_name))

            if not self.validate_images(validations):
                return -1

            if self.arguments['validate']:
                return 0

            tag = self.arguments['--tag']

            self.direct_build(image_name, self.arguments['SOURCE_DIR'], self.arguments['--incremental'],
                              self.arguments['--user'], tag, self.arguments['ENV_NAME=VALUE'])
        finally:
            self.docker_client.close()

class ImageValidationRequest:
    def __init__(self, description, image_name, validate_incremental=False):
        self.description = description
        self.image_name = image_name
        self.validate_incremental = validate_incremental

def main():
    builder = Builder()
    builder.main()

if __name__ == "__main__":
    sys.path.insert(0, '.')
    main()
