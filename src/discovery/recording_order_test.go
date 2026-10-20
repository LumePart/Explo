package discovery

import (
	"reflect"
	"testing"
)

// BUG-23: getTracks iterated the Recordings response map, so playlist
// order changed randomly between runs instead of following the ranking.
func TestOrderedRecordingMBIDs_KeepsRequestOrder(t *testing.T) {
	mk := func() Recordings {
		return Recordings{"c": {}, "a": {}, "b": {}}
	}
	got := orderedRecordingMBIDs([]string{"a", "b", "c"}, mk())
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ordered = %v, want %v", got, want)
	}
}

func TestOrderedRecordingMBIDs_DropsMissingRecordings(t *testing.T) {
	got := orderedRecordingMBIDs(
		[]string{"a", "missing", "b"},
		Recordings{"b": {}, "a": {}},
	)
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ordered = %v, want %v", got, want)
	}
}
