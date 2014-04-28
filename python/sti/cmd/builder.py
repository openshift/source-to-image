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
import subprocess
import re


class Builder(object):
    """
    Docker Source to Image(STI) is a tool for building reproducable Docker images.  STI produces ready-to-run images by
    injecting a user source into a docker image and preparing a new Docker image which incorporates the base image and
    built source, and is ready to use with `docker run`.  STI supports incremental builds which re-use previously
    downloaded dependencies, previously built artifacts, etc.

    Usage:
        sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG [--runtime-image=RUNTIME_IMAGE_TAG] [--clean]
            [--user=USERID] [--url=URL] [--timeout=TIMEOUT] [-e ENV_NAME=VALUE]... [-l LOG_LEVEL]
            [--dir=WORKING_DIR] [--push]
        sti validate BUILD_IMAGE_TAG [--runtime-image=RUNTIME_IMAGE_TAG] [--incremental] [--url=URL]
            [--timeout=TIMEOUT] [-l LOG_LEVEL]
        sti --help

    Arguments:
        BUILD_IMAGE_TAG        Tag for the Docker image which provides the build and runtime for the application.
        SOURCE_DIR             Directory or GIT repository containing your application sources.
        APP_IMAGE_TAG          Tag for the Docker image which is created by STI. In the case of incremental
                               builds, this tag is also used to identify the previous build of the application.


    Options:
        --runtime-image=RUNTIME_IMAGE_TAG   Tag which identifies an optional Docker image with runtime components but
                                            none of the build dependencies. If provided, the application will be built
                                            with BUILD_IMAGE_TAG and the binaries will be extracted and installed on
                                            the runtime image.
        --clean                             Do a clean build, ie. do not perform an incremental build.
        --dir=WORKING_DIR                   Directory where Dockerfiles and other support scripts are created.
                                            (Default: temp dir)
        -l LOG_LEVEL                        Logging level. Default: INFO
        --timeout=TIMEOUT                   Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                       Perform the build as specified user.
        --url=URL                           Connect to docker at the specified url Default: $DOCKER_HOST or unix://var/run/docker.sock
        --help                              Print this help message.
    """
    def __init__(self):
        self.arguments = docopt.docopt(Builder.__doc__)

        log_level = self.arguments['-l'] or "INFO"
        numeric_level = getattr(logging, log_level.upper(), None)
        if not isinstance(numeric_level, int):
            logging.warn("Invalid log level %s. Defaulting to INFO", log_level)
            numeric_level = logging.INFO
        logging.basicConfig(level=numeric_level)
        self.logger = logging.getLogger(__name__)

        if self.arguments['--url'] is not None:
            self.docker_url = self.arguments['--url']
        else:
            self.docker_url = os.getenv('DOCKER_HOST', 'unix://var/run/docker.sock')

        # these two checks should be done by the python docker client ...
        if self.docker_url.startswith('tcp:'):
            self.docker_url = self.docker_url.replace('tcp:', 'http:')
        if self.docker_url == 'http://':
            self.docker_url = 'http://127.0.0.1:4243'

        try:
            self.timeout = float(self.arguments['--timeout'])
        except TypeError:
            self.timeout = 120

        self.docker_client = docker.Client(base_url=self.docker_url, timeout=self.timeout)
        server_version = self.docker_client.version()
        self.logger.debug("Connected to Docker server version %s. Server linux kernel: %s",
                          server_version['Version'], server_version['KernelVersion'])

    def check_file_exists(self, container_id, file_path):
        try:
            self.docker_client.copy(container_id, file_path)
            return True
        except docker.APIError as e:
            return False

    def pull_image(self, image_name):
        if not self.is_image_in_local_registry(image_name):
            self.docker_client.pull(image_name)
        else:
            self.logger.debug("Image %s is available in local registry", image_name)

    def is_image_in_local_registry(self, image_name):
        images = self.docker_client.images(image_name)
        self.logger.debug("Checking if %s found. Result: %s", image_name, len(images) != 0)
        return len(images) != 0

    def push_image(self, image_name):
        images = self.docker_client.images(image_name)
        if len(images) == 0:
            raise "Image %s not found in local registry. Unable to push." % image_name
        else:
            self.logger.debug("Image %s is available in local registry. Pushing to remote." % image_name)
            self.docker_client.push(image_name)

    def create_container(self, image_name):
        try:
            container = self.docker_client.create_container(image_name, command='/bin/true')
            container_id = container['Id']
            self.docker_client.start(container_id)
            exitcode = self.docker_client.wait(container_id)
            time.sleep(1)

            return container_id
        except docker.APIError as e:
            self.logger.critical("Error while creating container for image %s. %s", image_name, e.explanation)
            return None

    def remove_container(self, container_id):
        self.docker_client.remove_container(container_id)

    def validate_images(self, requests=[]):
        for request in requests:
            image_name = request.image_name
            self.pull_image(image_name)
            container_id = self.create_container(image_name)

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

        image = self.docker_client.inspect_image(images[0]['Id'])

        if image['config']['Entrypoint']:
            self.logger.critical("Image %s has a configured Entrypoint and is incompatible with sti", image_name)
            return False

        required_files = ['/usr/bin/prepare', '/usr/bin/run']
        if validate_incremental:
            required_files += ['/usr/bin/save-artifacts']

        valid_image = self.validate_required_files(container_id, required_files)

        if valid_image:
            self.logger.info("%s passes source image validation", image_name)

        return valid_image

    def validate_required_files(self, container_id, required_files=[]):
        valid_image = True

        for f in required_files:
            if not self.check_file_exists(container_id, f):
                valid_image = False
                self.logger.critical("Invalid image: file %s is missing.", f)

        return valid_image

    def detect_incremental_build(self, image_name):
        container_id = self.create_container(image_name)

        try:
            result = self.check_file_exists(container_id, '/usr/bin/save-artifacts')
            self.remove_container(container_id)

            return result
        except docker.APIError as e:
            self.logger.critical("Error while detecting whether image %s supports incremental build" % image_name)
            return False

    def prepare_source_dir(self, source, target_source_dir):
        if re.match('^(http(s?)|git|file)://', source):
            git_clone_cmd = "git clone --quiet %s %s" % (source, target_source_dir)
            try:
                self.logger.info("Fetching %s", source)
                subprocess.check_output(git_clone_cmd, stderr=subprocess.STDOUT, shell=True)
            except subprocess.CalledProcessError as e:
                self.logger.critical("%s command failed (%i)", git_clone_cmd, e.returncode)
                return False
        else:
            shutil.copytree(source, target_source_dir)

    def save_artifacts(self, image_name, target_dir):
        self.logger.info("Saving data from image %s for incremental build", image_name)
        container = self.docker_client.create_container(image_name,
                                                        ["/usr/bin/save-artifacts"],
                                                        volumes={"/usr/artifacts": {}})
        container_id = container['Id']
        self.docker_client.start(container_id, binds={target_dir: "/usr/artifacts"})
        exitcode = self.docker_client.wait(container_id)
        # TODO: error handling
        self.logger.debug(self.docker_client.logs(container_id))
        time.sleep(1)
        self.docker_client.remove_container(container_id)

    def build_deployable_image(self, image_name, context_dir, tag, env_str, incremental=False):
        with open(os.path.join(context_dir, 'Dockerfile'), 'w+') as docker_file:
            docker_file.write("FROM %s\n" % image_name)
            docker_file.write('ADD ./src /usr/src/\n')
            if incremental:
                docker_file.write('ADD ./artifacts /usr/artifacts/\n')
            for env in env_str:
                env = env.split("=")
                name = env[0]
                value = env[1]
                docker_file.write("ENV %s %s\n" % (name, value))
            docker_file.write('RUN /usr/bin/prepare\n')
            docker_file.write('CMD /usr/bin/run\n')

        self.logger.info("Building new docker image")
        img, logs = self.docker_client.build(tag=tag, path=context_dir, rm=True)
        self.logger.info("Build logs:\n%s", logs)

        return img

    def build(self, working_dir, image_name, source_dir, incremental_build, user_id, tag, env_str):
        build_dir = working_dir or tempfile.mkdtemp()

        try:
            if incremental_build:
                artifact_tmp_dir = os.path.join(build_dir, 'artifacts')
                os.mkdir(artifact_tmp_dir)
                self.save_artifacts(tag, artifact_tmp_dir)

            build_context_source = os.path.join(build_dir, 'src')
            self.prepare_source_dir(source_dir, build_context_source)
            img = self.build_deployable_image(image_name, build_dir, tag, env_str, incremental_build)

            if img is not None:
                built_image_name = tag or img
                self.logger.info("%s Built image %s %s", Fore.GREEN, built_image_name, Fore.RESET)
            else:
                self.logger.critical("%s STI build failed. %s", Fore.RED, Fore.RESET)

        finally:
            if not working_dir:
                shutil.rmtree(build_dir)
            pass

    def extended_build(self, working_dir, build_image, runtime_image, source_dir, incremental_build, user_id, tag, app_build_tag, env_str):
        build_dir = working_dir or tempfile.mkdtemp()

        builder_build_dir = os.path.join(build_dir, 'build')
        runtime_build_dir = os.path.join(build_dir, 'runtime')
        previous_build_volume = os.path.join(builder_build_dir, 'last_build_artifacts')
        input_source_dir = os.path.join(builder_build_dir, 'src')
        output_source_dir = os.path.join(runtime_build_dir, 'src')

        os.mkdir(builder_build_dir)
        os.mkdir(runtime_build_dir)
        os.mkdir(previous_build_volume)
        os.mkdir(output_source_dir)

        build_container_id = None
        try:
            self.logger.debug("Incremental build: %s", incremental_build)
            if incremental_build:
                self.pull_image(app_build_tag)
                self.save_artifacts(app_build_tag, previous_build_volume)

            volumes = {'/usr/artifacts': {}, '/usr/src': {}, '/usr/build': {}}
            bind_mounts = {
                previous_build_volume: '/usr/artifacts',
                input_source_dir: '/usr/src',
                output_source_dir: '/usr/build'
            }
            self.prepare_source_dir(source_dir, input_source_dir)
            build_container = self.docker_client.create_container(build_image, '/usr/bin/prepare', volumes=volumes)
            build_container_id = build_container['Id']
            self.docker_client.start(build_container_id, binds=bind_mounts)
            exitcode = self.docker_client.wait(build_container_id)
            self.logger.debug(self.docker_client.logs(build_container_id))

            if exitcode != 0:
                self.logger.error("Unable to build application")
                raise "Unable to build container"

            img = self.build_deployable_image(runtime_image, runtime_build_dir, tag, env_str)
            if img is not None:
                built_image_name = tag or img
                build_container_img = self.docker_client.commit(build_container_id)
                self.docker_client.tag(build_container_img['Id'], app_build_tag)
                self.logger.info("%s Built build-image %s %s", Fore.GREEN, app_build_tag, Fore.RESET)
                self.logger.info("%s Built image %s %s", Fore.GREEN, built_image_name, Fore.RESET)
            else:
                self.logger.critical("%s STI build failed. %s", Fore.RED, Fore.RESET)
        finally:
            if build_container_id != None:
              self.remove_container(build_container_id)
            if not working_dir:
                shutil.rmtree(builder_build_dir)
                shutil.rmtree(runtime_build_dir)
                shutil.rmtree(build_dir)

    def main(self):
        build_image = self.arguments['BUILD_IMAGE_TAG']
        runtime_image = self.arguments['--runtime-image']
        app_image = self.arguments['APP_IMAGE_TAG']
        app_build_tag = "%s-build" % app_image
        source = self.arguments['SOURCE_DIR']
        user = self.arguments['--user']
        env_str = self.arguments['ENV_NAME=VALUE']
        is_incremental = not self.arguments['--clean']
        working_dir = self.arguments['--dir']
        should_push = self.arguments['--push']


        validations = []

        try:
            if runtime_image:
                if is_incremental:
                    self.pull_image(app_build_tag)
                    is_incremental = self.is_image_in_local_registry(app_build_tag)
                if self.arguments['validate']:
                    validations.append(ImageValidationRequest('Runtime image', runtime_image, False))
                    validations.append(ImageValidationRequest('Build image', build_image, True))
                    self.validate_images(validations)
                elif self.arguments['build']:
                    self.extended_build(working_dir, build_image, runtime_image, source, is_incremental, user, app_image, app_build_tag, env_str)
            else:
                if is_incremental:
                    self.pull_image(app_image)
                    is_incremental = self.is_image_in_local_registry(app_image)
                if self.arguments['validate']:
                    validations.append(ImageValidationRequest('Target image', build_image,
                                                              self.arguments['--incremental']))
                    self.validate_images(validations)
                elif self.arguments['build']:
                    self.build(working_dir, build_image, source, is_incremental, user, app_image, env_str)
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
