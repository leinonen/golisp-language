package core

import "testing"

func TestNamespacesLoad(t *testing.T) {
	nss, err := Namespaces()
	if err != nil {
		t.Fatalf("core failed to load: %v", err)
	}
	for _, ns := range []string{"str", "sys", "core"} {
		if _, ok := nss[ns]; !ok {
			t.Errorf("missing %q namespace", ns)
		}
	}
	str, ok := nss["str"]
	if !ok {
		t.Fatal("missing str namespace")
	}
	want := map[string]bool{"upper": false, "blank?": false, "join": false, "split": false}
	for _, fn := range str.Funcs {
		if _, tracked := want[fn.Name]; tracked {
			want[fn.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("str namespace missing expected fn %q", name)
		}
	}
}
