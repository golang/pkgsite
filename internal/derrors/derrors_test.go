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
		wantType                       ErrorType
	}{
		{
			label:      "identifies not found errors",
			err:        NotFound("couldn't find it"),
			isNotFound: true,
			wantType:   NotFoundType,
		}, {
			label:              "identifies invalid argument errors",
			err:                InvalidArgument("bad arguments"),
			isInvalidArguments: true,
			wantType:           InvalidArgumentType,
		}, {
			label:    "doesn't identify an unknown error",
			err:      errors.New("bad"),
			wantType: UncategorizedErrorType,
		}, {
			label: "doesn't identify a nil error",
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			if got := IsNotFound(test.err); got != test.isNotFound {
				t.Errorf("IsNotFound(%v) = %t, want %t", test.err, got, test.isNotFound)
			}
			if got := IsInvalidArgument(test.err); got != test.isInvalidArguments {
				t.Errorf("IsInvalidArguments(%v) = %t, want %t", test.err, got, test.isInvalidArguments)
			}

			if got := Type(test.err); got != test.wantType {
				t.Errorf("Type(%v) = %v, want %v", test.err, got, test.wantType)
			}
		})
	}
}
