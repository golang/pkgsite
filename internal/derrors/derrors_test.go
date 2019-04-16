package derrors

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	tests := []struct {
		label                          string
		err                            error
		isNotFound, isInvalidArguments bool
	}{
		{
			label:      "identifies not found errors",
			err:        NotFound("couldn't find it"),
			isNotFound: true,
		}, {
			label:              "identifies invalid argument errors",
			err:                InvalidArguments("bad arguments"),
			isInvalidArguments: true,
		}, {
			label: "doesn't identify an unknown error",
			err:   errors.New("bad"),
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			if got := IsNotFound(test.err); got != test.isNotFound {
				t.Errorf("IsNotFound(%v) = %t, want %t", test.err, got, test.isNotFound)
			}
			if got := IsInvalidArguments(test.err); got != test.isInvalidArguments {
				t.Errorf("IsInvalidArguments(%v) = %t, want %t", test.err, got, test.isInvalidArguments)
			}
		})
	}
}
