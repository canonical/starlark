package starlark_test

import (
	"testing"

	"github.com/canonical/starlark/starlark"
	"github.com/canonical/starlark/startest"
)

func TestUniverseSafeties(t *testing.T) {
	for name, value := range starlark.Universe {
		builtin, ok := value.(*starlark.Builtin)
		if !ok {
			continue
		}

		if safety, ok := (*starlark.UniverseSafeties)[name]; !ok {
			t.Errorf("builtin %s has no safety declaration", name)
		} else if actualSafety := builtin.Safety(); actualSafety != safety {
			t.Errorf("builtin %s has incorrect safety: expected %v but got %v", name, safety, actualSafety)
		}
	}

	for name, _ := range *starlark.UniverseSafeties {
		if _, ok := starlark.Universe[name]; !ok {
			t.Errorf("safety declared for non-existent builtin: %s", name)
		}
	}
}

func TestBytesMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "bytes", starlark.BytesMethods, starlark.BytesMethodSafeties)
}

func TestDictMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "dict", starlark.DictMethods, starlark.DictMethodSafeties)
}

func TestListMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "list", starlark.ListMethods, starlark.ListMethodSafeties)
}

func TestStringMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "string", starlark.StringMethods, starlark.StringMethodSafeties)
}

func TestSetMethodSafeties(t *testing.T) {
	testBuiltinSafeties(t, "set", starlark.SetMethods, starlark.SetMethodSafeties)
}

func testBuiltinSafeties(t *testing.T, recvName string, builtins map[string]*starlark.Builtin, safeties map[string]starlark.Safety) {
	for name, builtin := range builtins {
		if safety, ok := safeties[name]; !ok {
			t.Errorf("builtin %s.%s has no safety declaration", recvName, name)
		} else if actual := builtin.Safety(); actual != safety {
			t.Errorf("builtin %s.%s has incorrect safety: expected %v but got %v", name, recvName, safety, actual)
		}
	}

	for name, _ := range safeties {
		if _, ok := builtins[name]; !ok {
			t.Errorf("safety declared for non-existent builtin %s.%s", recvName, name)
		}
	}
}

func TestAbsAllocs(t *testing.T) {
}

func TestAnyAllocs(t *testing.T) {
}

func TestAllAllocs(t *testing.T) {
}

func TestBoolAllocs(t *testing.T) {
}

func TestBytesAllocs(t *testing.T) {
}

func TestChrAllocs(t *testing.T) {
}

func TestDictAllocs(t *testing.T) {
}

func TestDirAllocs(t *testing.T) {
}

func TestEnumerateAllocs(t *testing.T) {
}

func TestFailAllocs(t *testing.T) {
}

func TestFloatAllocs(t *testing.T) {
}

func TestGetattrAllocs(t *testing.T) {
}

func TestHasattrAllocs(t *testing.T) {
}

func TestHashAllocs(t *testing.T) {
}

func TestIntAllocs(t *testing.T) {
}

func TestLenAllocs(t *testing.T) {
}

func TestListAllocs(t *testing.T) {
}

func TestMaxAllocs(t *testing.T) {
}

func TestMinAllocs(t *testing.T) {
}

func TestOrdAllocs(t *testing.T) {
}

func TestPrintAllocs(t *testing.T) {
}

func TestRangeAllocs(t *testing.T) {
}

func TestReprAllocs(t *testing.T) {
}

func TestReversedAllocs(t *testing.T) {
}

func TestSetAllocs(t *testing.T) {
}

func TestSortedAllocs(t *testing.T) {
}

func TestStrAllocs(t *testing.T) {
}

func TestTupleAllocs(t *testing.T) {
}

func TestTypeAllocs(t *testing.T) {
}

func TestZipAllocs(t *testing.T) {
}

func TestBytesElemsAllocs(t *testing.T) {
}

func TestDictClearAllocs(t *testing.T) {
}

func TestDictGetAllocs(t *testing.T) {
}

func TestDictItemsAllocs(t *testing.T) {
}

func TestDictKeysAllocs(t *testing.T) {
}

func TestDictPopAllocs(t *testing.T) {
}

func TestDictPopitemAllocs(t *testing.T) {
}

func TestDictSetdefaultAllocs(t *testing.T) {
}

func TestDictUpdateAllocs(t *testing.T) {
}

func TestDictValuesAllocs(t *testing.T) {
}

func TestListAppendAllocs(t *testing.T) {
}

func TestListClearAllocs(t *testing.T) {
}

func TestListExtendAllocs(t *testing.T) {
}

func TestListIndexAllocs(t *testing.T) {
}

func TestListInsertAllocs(t *testing.T) {
}

func TestListPopAllocs(t *testing.T) {
}

func TestListRemoveAllocs(t *testing.T) {
}

func TestStringCapitalizeAllocs(t *testing.T) {
}

func TestStringCodepoint_ordsAllocs(t *testing.T) {
}

func TestStringCodepointsAllocs(t *testing.T) {
}

func TestStringCountAllocs(t *testing.T) {
}

func TestStringElem_ordsAllocs(t *testing.T) {
}

func TestStringElemsAllocs(t *testing.T) {
}

func TestStringEndswithAllocs(t *testing.T) {
}

func TestStringFindAllocs(t *testing.T) {
}

func TestStringFormatAllocs(t *testing.T) {
}

func TestStringIndexAllocs(t *testing.T) {
}

func TestStringIsalnumAllocs(t *testing.T) {
}

func TestStringIsalphaAllocs(t *testing.T) {
}

func TestStringIsdigitAllocs(t *testing.T) {
}

func TestStringIslowerAllocs(t *testing.T) {
}

func TestStringIsspaceAllocs(t *testing.T) {
}

func TestStringIstitleAllocs(t *testing.T) {
}

func TestStringIsupperAllocs(t *testing.T) {
}

func TestStringJoinAllocs(t *testing.T) {
}

func TestStringLowerAllocs(t *testing.T) {
}

func TestStringLstripAllocs(t *testing.T) {
	string_lstrip, _ := starlark.String("    library").Attr("lstrip")
	if string_lstrip == nil {
		t.Errorf("No such method: string.lstrip")
		return
	}

	t.Run("with-cutset=no", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_lstrip, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("with-cutset=yes", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				cutset := starlark.String("lint ")
				result, err := starlark.Call(thread, string_lstrip, starlark.Tuple{cutset}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringPartitionAllocs(t *testing.T) {
}

func TestStringRemoveprefixAllocs(t *testing.T) {
}

func TestStringRemovesuffixAllocs(t *testing.T) {
}

func TestStringReplaceAllocs(t *testing.T) {
}

func TestStringRfindAllocs(t *testing.T) {
}

func TestStringRindexAllocs(t *testing.T) {
}

func TestStringRpartitionAllocs(t *testing.T) {
}

func TestStringRsplitAllocs(t *testing.T) {
}

func TestStringRstripAllocs(t *testing.T) {
	string_lstrip, _ := starlark.String("bamboo   ").Attr("rstrip")
	if string_lstrip == nil {
		t.Errorf("No such method: string.rstrip")
		return
	}

	t.Run("with-cutset=no", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_lstrip, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("with-cutset=yes", func(t *testing.T) {
		st := startest.From(t)
		st.RequireSafety(starlark.MemSafe)
		st.SetMaxAllocs(16)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				cutset := starlark.String("roots ")
				result, err := starlark.Call(thread, string_lstrip, starlark.Tuple{cutset}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringSplitAllocs(t *testing.T) {
}

func TestStringSplitlinesAllocs(t *testing.T) {
}

func TestStringStartswithAllocs(t *testing.T) {
}

func TestStringStripAllocs(t *testing.T) {
	string_strip, _ := starlark.String("    airship    ").Attr("strip")
	if string_strip == nil {
		t.Errorf("No such method: string.strip")
		return
	}

	t.Run("with-cutset=no", func(t *testing.T) {
		st := startest.From(t)
		st.SetMaxAllocs(16)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				result, err := starlark.Call(thread, string_strip, nil, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})

	t.Run("with-cutset=yes", func(t *testing.T) {
		st := startest.From(t)
		st.SetMaxAllocs(16)
		st.RequireSafety(starlark.MemSafe)
		st.RunThread(func(thread *starlark.Thread) {
			for i := 0; i < st.N; i++ {
				cutset := starlark.String("paint ")
				result, err := starlark.Call(thread, string_strip, starlark.Tuple{cutset}, nil)
				if err != nil {
					st.Error(err)
				}
				st.KeepAlive(result)
			}
		})
	})
}

func TestStringTitleAllocs(t *testing.T) {
}

func TestStringUpperAllocs(t *testing.T) {
}

func TestSetUnionAllocs(t *testing.T) {
}
