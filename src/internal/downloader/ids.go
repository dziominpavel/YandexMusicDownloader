package downloader

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// idString renders a JSON id as a plain decimal string. The API mixes
// numeric and string ids; decoding into any turns numbers into float64,
// and fmt.Sprint on a large float64 produces scientific notation
// ("6.73558e+06"), which breaks source URLs and tags.
func idString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return ""
		}
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return fmt.Sprint(x)
	}
}
