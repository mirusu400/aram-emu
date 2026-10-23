package mobile

import (
	"fmt"
	"math"

	"github.com/mirusu400/aram-frontend/frontend"
)

func hostEventWithWords(
	kind frontend.HostEventKind,
	value0, value1, value2, value3 int64,
) (frontend.HostEvent, error) {
	values := [...]int64{value0, value1, value2, value3}
	var words [4]uint32
	for index, value := range values {
		if value < 0 || value > math.MaxUint32 {
			return frontend.HostEvent{}, fmt.Errorf(
				"host event word %d is outside 0..%d",
				index,
				uint64(math.MaxUint32),
			)
		}
		words[index] = uint32(value)
	}
	return frontend.HostEvent{Kind: kind, Words: words}, nil
}
