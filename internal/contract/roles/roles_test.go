// Levier - role set tests.
// Copyright 2026 The Levier Authors.
package roles

import (
	"reflect"
	"testing"
)

func TestParseAllRoles(t *testing.T) {
	set, err := Parse([]string{All})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !set.IsAll() {
		t.Fatal("all role did not expand")
	}
	if !set.Has(RuntimeGateway) || !set.Has(AppGateway) {
		t.Fatalf("expanded set = %+v", set.List())
	}
	if !reflect.DeepEqual(set.List(), []string{Control, API, RuntimeGateway, AppGateway}) {
		t.Fatalf("list = %v", set.List())
	}
}

func TestParseSubsetAndUnknown(t *testing.T) {
	set, err := Parse([]string{RuntimeGateway, API})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if set.IsAll() || !set.Has(API) || set.Has(Control) {
		t.Fatalf("subset = %+v", set.List())
	}
	if _, err := Parse([]string{"worker"}); err == nil {
		t.Fatal("unknown role was accepted")
	}
	if _, err := Parse(nil); err == nil {
		t.Fatal("empty roles were accepted")
	}
}
