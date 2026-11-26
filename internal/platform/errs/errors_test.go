package errs

import "testing"

func TestCodeMetaIsComplete(t *testing.T) {
	for code, meta := range codeMetaByCode {
		if meta.status < 400 {
			t.Fatalf("%s status = %d, want >= 400", code, meta.status)
		}
		if got := HTTPStatus(code); got != meta.status {
			t.Fatalf("%s HTTPStatus = %d, want %d", code, got, meta.status)
		}
	}
	if got := HTTPStatus(Code("UNKNOWN_CODE")); got != 500 {
		t.Fatalf("unknown code status = %d, want 500", got)
	}
}

func TestCodeMetaPins(t *testing.T) {
	cases := []struct {
		code      Code
		status    int
		retryable bool
	}{
		{CodeToolTimeout, 504, true},
		{CodeToolDenied, 403, false},
		{CodeAppUnreachable, 503, true},
		{CodeDependencyUnavailable, 503, true},
		{CodeRequestTooLarge, 413, false},
		{CodeQueueUnavailable, 503, true},
		{CodeInternal, 500, false},
	}
	for _, tc := range cases {
		meta := codeMetaByCode[tc.code]
		if meta.status != tc.status || meta.retryable != tc.retryable {
			t.Fatalf("%s meta = %+v, want status=%d retryable=%v", tc.code, meta, tc.status, tc.retryable)
		}
	}
}
