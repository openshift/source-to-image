package commonutils

import (
	"encoding/json"
	"io"

	"github.com/Microsoft/opengcs/service/gcs/gcserr"
)

// UnmarshalJSONWithHresult unmarshals the given data into the given interface, and
// wraps any error returned in an HRESULT error.
func UnmarshalJSONWithHresult(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
	}
	return nil
}

// DecodeJSONWithHresult decodes the JSON from the given reader into the given
// interface, and wraps any error returned in an HRESULT error.
func DecodeJSONWithHresult(r io.Reader, v interface{}) error {
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
	}
	return nil
}
