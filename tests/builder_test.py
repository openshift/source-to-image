import docker
import tempfile
import shutil
import subprocess
import time

def invoke_subprocess(command):
    return subprocess.Popen(command, shell=True, bufsize=0)
 
def run_command(command):
    print("Running command \"%s\"" % command)
    p = invoke_subprocess(command)
    p.wait()
    return p.returncode

def make_docker_client(url='unix://var/run/docker.sock', timeout=60):
    docker_client = docker.Client(base_url=url, timeout=timeout)	
    server_version = docker_client.version()
    assert (server_version is not None), "Couldn't connect to Docker"
    return docker_client

def build_source_image(image_name, docker_client):
    print("Building wharfie source image %s" % image_name)
    image_path = "test_sources/images/%s" % image_name
    test_image_tag = "wharfie-test/%s" % image_name
    img, logs = docker_client.build(path=image_path, rm=True, tag=test_image_tag)
    print("Build logs: %s" % logs)
    assert (img is not None), "Source image docker build failed"
    return test_image_tag

def wharfie_build(source_image, application_source, tag):
    exitcode = run_command("wharfie build %s %s --tag %s" % (source_image, application_source, tag))
    assert exitcode == 0, 'Wharfie build failed'

def run_wharfie_product(image_name, docker_client):
    container = docker_client.create_container(image_name)
    container_id = container['Id']
    assert (container_id is not None), "Couldn't create a container from wharfie build product"

    docker_client.start(container_id)
    exitcode = docker_client.wait(container)
    assert exitcode == 0

    return container_id

def check_file_exists(container_id, file_path, docker_client):
    try:
        docker_client.copy(container_id, file_path)
        return True
    except docker.APIError as e:
        print("file %s does not exist in %s" % (file_path, container_id))
        return False

def check_basic_build_state(container_id, docker_client):
    assert check_file_exists(container_id, '/wharfie-fake/prepare-invoked', docker_client)
    assert check_file_exists(container_id, '/wharfie-fake/run-invoked', docker_client)

def test_basic_build():
    wharfie_build_tag = 'test/wharfie-app' 
    app_source = 'test_sources/applications/html'

    docker_client = make_docker_client()
    test_image_tag = build_source_image('wharfie-fake', docker_client)

    wharfie_build(test_image_tag, app_source, wharfie_build_tag)
    container_id = run_wharfie_product(wharfie_build_tag, docker_client)
    check_basic_build_state(container_id, docker_client)

