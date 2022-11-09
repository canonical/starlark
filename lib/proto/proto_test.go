package proto_test

import (
	"testing"

	"github.com/canonical/starlark/lib/proto"
	"github.com/canonical/starlark/starlark"
)

func TestModuleSafeties(t *testing.T) {
	for name, value := range proto.Module.Members {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := proto.Safeties[name]; !ok {
			t.Errorf("builtin proto.%s has no safety declaration", name)
		} else if actual := builtin.Safety(); actual != safety {
			t.Errorf("builtin proto.%s has incorrect safety: expected %v but got %v", name, safety, actual)
		}
	}

	for name, _ := range proto.Safeties {
		if _, ok := proto.Module.Members[name]; !ok {
			t.Errorf("safety declared for non-existent builtin proto.%s", name)
		}
	}
}

func TestProtoFileAllocs(t *testing.T) {
}

func TestProtoHasAllocs(t *testing.T) {
}

func TestProtoMarshalAllocs(t *testing.T) {
}

func TestProtoMarshalTextAllocs(t *testing.T) {
}

func TestProtoSetFieldAllocs(t *testing.T) {
}

func TestProtoGetFieldAllocs(t *testing.T) {
}

func TestProtoUnmarshalAllocs(t *testing.T) {
}

func TestProtoUnmarshalTextAllocs(t *testing.T) {
}
