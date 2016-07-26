/*
Copyright 2014 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package framework

import (
	"fmt"
	"math/rand"

	etcd "github.com/coreos/etcd/client"
	"github.com/golang/glog"
	"golang.org/x/net/context"

	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/storage"
	etcdstorage "k8s.io/kubernetes/pkg/storage/etcd"
	"k8s.io/kubernetes/pkg/storage/etcd/etcdtest"
)

// If you need to start an etcd instance by hand, you also need to insert a key
// for this check to pass (*any* key will do, eg:
//curl -L http://127.0.0.1:4001/v2/keys/message -XPUT -d value="Hello world").
func init() {
	RequireEtcd()
}

func NewEtcdClient() etcd.Client {
	cfg := etcd.Config{
		Endpoints: []string{"http://127.0.0.1:4001"},
	}
	client, err := etcd.New(cfg)
	if err != nil {
		glog.Fatalf("unable to connect to etcd for testing: %v", err)
	}
	return client
}

func NewAutoscalingEtcdStorage(client etcd.Client) storage.Interface {
	if client == nil {
		client = NewEtcdClient()
	}
	return etcdstorage.NewEtcdStorage(client, testapi.Autoscaling.Codec(), etcdtest.PathPrefix(), false, etcdtest.DeserializationCacheSize)
}

func NewBatchEtcdStorage(client etcd.Client) storage.Interface {
	if client == nil {
		client = NewEtcdClient()
	}
	return etcdstorage.NewEtcdStorage(client, testapi.Batch.Codec(), etcdtest.PathPrefix(), false, etcdtest.DeserializationCacheSize)
}

func NewAppsEtcdStorage(client etcd.Client) storage.Interface {
	if client == nil {
		client = NewEtcdClient()
	}
	return etcdstorage.NewEtcdStorage(client, testapi.Apps.Codec(), etcdtest.PathPrefix(), false, etcdtest.DeserializationCacheSize)
}

func NewExtensionsEtcdStorage(client etcd.Client) storage.Interface {
	if client == nil {
		client = NewEtcdClient()
	}
	return etcdstorage.NewEtcdStorage(client, testapi.Extensions.Codec(), etcdtest.PathPrefix(), false, etcdtest.DeserializationCacheSize)
}

func RequireEtcd() {
	if _, err := etcd.NewKeysAPI(NewEtcdClient()).Get(context.TODO(), "/", nil); err != nil {
		glog.Fatalf("unable to connect to etcd for testing: %v", err)
	}
}

func WithEtcdKey(f func(string)) {
	prefix := fmt.Sprintf("/test-%d", rand.Int63())
	defer etcd.NewKeysAPI(NewEtcdClient()).Delete(context.TODO(), prefix, &etcd.DeleteOptions{Recursive: true})
	f(prefix)
}

// DeleteAllEtcdKeys deletes all keys from etcd.
// TODO: Instead of sprinkling calls to this throughout the code, adjust the
// prefix in etcdtest package; then just delete everything once at the end
// of the test run.
func DeleteAllEtcdKeys() {
	glog.Infof("Deleting all etcd keys")
	keysAPI := etcd.NewKeysAPI(NewEtcdClient())
	keys, err := keysAPI.Get(context.TODO(), "/", nil)
	if err != nil {
		glog.Fatalf("Unable to list root etcd keys: %v", err)
	}
	for _, node := range keys.Node.Nodes {
		if _, err := keysAPI.Delete(context.TODO(), node.Key, &etcd.DeleteOptions{Recursive: true}); err != nil {
			glog.Fatalf("Unable delete key: %v", err)
		}
	}

}
