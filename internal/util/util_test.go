package util

import "testing"

func TestFormatNumber(t *testing.T) {
	cases := map[string]string{
		"500000": "500 000", "6287.726": "6 287.726", "6287,726": "6 287.726",
		"1 000": "1 000", "0.1234567": "0.123457", "abc": "abc", "100.0": "100",
	}
	for in, want := range cases {
		if got := FormatNumber(in); got != want {
			t.Errorf("FormatNumber(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestPyFloatAndFmt2(t *testing.T) {
	if PyFloat(80) != "80.0" || PyFloat(79.52) != "79.52" {
		t.Error(PyFloat(80), PyFloat(79.52))
	}
	if Fmt2(10) != "10" || Fmt2(12.5) != "12.5" || Fmt2(0) != "0" {
		t.Error(Fmt2(10), Fmt2(12.5), Fmt2(0))
	}
	if SafeNamePart("ОсОО «Ромашка»", 10, "CL") != "ОсОО_Ромаш" {
		t.Error(SafeNamePart("ОсОО «Ромашка»", 10, "CL"))
	}
}
