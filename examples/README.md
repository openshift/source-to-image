Creating a basic S2I builder image  
---

### Getting started  

Directory skeleton:  

* `Dockerfile` – standard Dockerfile where we’ll define the base builder image.  
* `assemble` - responsible for building the application.  
* `run` - responsible for running the application.  
* `save-artifacts` - optional script for incremental builds that save built artifacts.  
* `usage` - optional script responsible for printing the usage of the builder.  

The first step is to create a `Dockerfile` that installs all the necessary tools and libraries that are needed to build and run our application.  
You can find an example of the Dockerfile [`here`](nginx-app/Dockerfile).   

The next step is to create an `assemble` script that will, for example, build python modules, bundle install our required components or setup application specific configuration based on the logic we define in it. We can specify a way to restore any saved artifacts from the previous image. In [`this`](nginx-app/assemble) example it will only copy our index.html over the default one.  

Now we can create our `run` script that will start the application. You can see an example [`here`](nginx-app/run).

Optionally we can also specify a `save-artifacts` script, which allows a new build to reuses content from a previous version of the application image.
An example can be found [`here`](nginx-app/save-artifacts).  

We can provide some help to the user on how to use it as a base for an application image via the `usage` script. An example can be found [`here`](nginx-app/usage).

Make sure all the scripts are runnable `chmod +x assemble run save-artifacts usage`

The next step is to create the builder image. In the nginx-app directory issue `docker build -t nginx-centos7 .`  
This will create a builder image from the current Dockerfile.

Once the builder image is done, the user can issue `s2i usage nginx-centos7` which will print out the help info that was defined in our `usage` script.

The next step is to create `the application image`. We will create this with the content from the source directory `test` from this repo. In this source directory we have only one file for this example, `index.html`.

```
s2i build test/ nginx-centos7 nginx-app
---> Building and installing application from source...
```
All the logic defined previously in the `assemble` script will now be executed thus compiling your assets or setting up application specific configuration.

Running the application image is as simple as invoking the docker run command:
`docker run -d -p 8080:8080 nginx-app`

Now you should be able to access a static web page served by our newly created application image on [http://localhost:8080](http://localhost:8080).

If we want to rebuild the application with the saved artifacts, then we can do:
```
s2i build --incremental=true test/ nginx-centos7 nginx-app
---> Restoring build artifacts...
---> Building and installing application from source...
```
This will run the `save-artifacts` script that has the code which will save your artifacts from the previously built application image, and then inject those artifacts into the new image according to the logic you specified in the `assemble` script. 

####Best Practices on Image Creation
Please also read [https://github.com/openshift/openshift-docs/blob/master/creating_images/guidelines.adoc](https://github.com/openshift/openshift-docs/blob/master/creating_images/guidelines.adoc) on best practices when creating a image. 
