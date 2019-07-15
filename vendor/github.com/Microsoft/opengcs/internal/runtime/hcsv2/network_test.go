// +build linux

package hcsv2

import (
	"context"
	"testing"

	"github.com/Microsoft/opengcs/service/gcs/prot"
)

func Test_getNetworkNamespace_NotExist(t *testing.T) {
	defer func() {
		err := removeNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns, err := getNetworkNamespace(t.Name())
	if err == nil {
		t.Fatal("expected error got nil")
	}
	if ns != nil {
		t.Fatalf("namespace should be nil, got: %+v", ns)
	}
}

func Test_getNetworkNamespace_PreviousExist(t *testing.T) {
	defer func() {
		err := removeNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns1 := getOrAddNetworkNamespace(t.Name())
	if ns1 == nil {
		t.Fatal("namespace ns1 should not be nil")
	}
	ns2, err := getNetworkNamespace(t.Name())
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
	if ns1 != ns2 {
		t.Fatalf("ns1 %+v != ns2 %+v", ns1, ns2)
	}
}

func Test_getOrAddNetworkNamespace_NotExist(t *testing.T) {
	defer func() {
		err := removeNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns := getOrAddNetworkNamespace(t.Name())
	if ns == nil {
		t.Fatalf("namespace should not be nil")
	}
}

func Test_getOrAddNetworkNamespace_PreviousExist(t *testing.T) {
	defer func() {
		err := removeNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()

	ns1 := getOrAddNetworkNamespace(t.Name())
	ns2 := getOrAddNetworkNamespace(t.Name())
	if ns1 != ns2 {
		t.Fatalf("ns1 %+v != ns2 %+v", ns1, ns2)
	}
}

func Test_removeNetworkNamespace_NotExist(t *testing.T) {
	err := removeNetworkNamespace(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("failed to remove non-existing ns with error: %v", err)
	}
}

func Test_removeNetworkNamespace_HasAdapters(t *testing.T) {
	defer func() {
		err := removeNetworkNamespace(context.Background(), t.Name())
		if err != nil {
			t.Errorf("failed to remove ns with error: %v", err)
		}
	}()
	nsOld := networkInstanceIDToName
	defer func() {
		networkInstanceIDToName = nsOld
	}()

	ns := getOrAddNetworkNamespace(t.Name())

	networkInstanceIDToName = func(ctx context.Context, id string, wait bool) (string, error) {
		return "/dev/sdz", nil
	}
	err := ns.AddAdapter(context.Background(), &prot.NetworkAdapterV2{ID: "test"})
	if err != nil {
		t.Fatalf("failed to add adapter: %v", err)
	}
	err = removeNetworkNamespace(context.Background(), t.Name())
	if err == nil {
		t.Fatal("should have failed to delete namespace with adapters")
	}
	err = ns.RemoveAdapter(context.Background(), "test")
	if err != nil {
		t.Fatalf("failed to remove adapter: %v", err)
	}
	err = removeNetworkNamespace(context.Background(), t.Name())
	if err != nil {
		t.Fatalf("should not have failed to delete empty namepace got: %v", err)
	}
}
