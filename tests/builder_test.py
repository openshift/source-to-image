import docker
import tempfile
import shutil
import subprocess
import time
 
def run_command(command):
    print("Running command \"%s\"" % command)
    p = subprocess.Popen(command, shell=True, bufsize=0)
    p.wait()
    return p.returncode

def make_docker_client(url='unix://var/run/docker.sock', timeout=60):
    docker_client = docker.Client(base_url=url, timeout=timeout)	
    server_version = docker_client.version()
    assert (server_version is not None), "Couldn't connect to Docker"
    return docker_client

class TestBuilder:
    def build_source_image(self, image_name):
        print("Building source image %s" % image_name)
        image_path = "test_sources/images/%s" % image_name
        test_image_tag = "sti-test/%s" % image_name
        img, logs = self.docker_client.build(path=image_path, rm=True, tag=test_image_tag)
        print("Build logs: %s" % logs)
        assert (img is not None), "Source image docker build failed"
        return test_image_tag

    def basic_build(self, source_image, application_source, tag):
        exitcode = run_command("sti build %s %s --tag %s" % (source_image, application_source, tag))
        assert exitcode == 0, 'build failed'

    def incremental_build(self, source_image, prev_image, application_source, tag):
        exitcode = run_command("sti build %s %s --tag %s --incremental %s" % (source_image, application_source, tag, prev_image))
        assert exitcode == 0, 'build failed'

    def run_sti_product(self, image_name):
        container = self.docker_client.create_container(image_name)
        container_id = container['Id']
        assert (container_id is not None), "Couldn't create a container from build product"

        self.docker_client.start(container_id)
        exitcode = self.docker_client.wait(container)
        assert exitcode == 0

        return container_id

    def check_file_exists(self, container_id, file_path):
        try:
            self.docker_client.copy(container_id, file_path)
            return True
        except docker.APIError as e:
            print("file %s does not exist in %s" % (file_path, container_id))
            return False

    def check_basic_build_state(self, container_id):
        assert self.check_file_exists(container_id, '/sti-fake/prepare-invoked')
        assert self.check_file_exists(container_id, '/sti-fake/run-invoked')

    def check_incremental_build_state(self, container_id):
        self.check_basic_build_state(container_id)
        assert self.check_file_exists(container_id, '/sti-fake/save-artifacts-invoked')

    def setup(self):
        self.docker_client = make_docker_client()

    def test_basic_build(self):
        sti_build_tag = 'test/sti-app'
        app_source = 'test_sources/applications/html'

        test_image_tag = self.build_source_image('sti-fake')
        self.basic_build(test_image_tag, app_source, sti_build_tag)
        container_id = self.run_sti_product(sti_build_tag)
        self.check_basic_build_state(container_id)

    def test_incremental_build(self):
        sti_build_tag = 'test/sti-incremental-app'
        app_source = 'test_sources/applications/html'

        test_image_tag = self.build_source_image('sti-fake')
        # use the sti-fake image as its own previous build
        self.incremental_build(test_image_tag, test_image_tag, app_source, sti_build_tag)
        container_id = self.run_sti_product(sti_build_tag)
        self.check_incremental_build_state(container_id)
