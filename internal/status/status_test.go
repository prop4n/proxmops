package status

import (
	"testing"
	"time"
)

func TestNewStoreEmptySnapshot(t *testing.T) {
	snap := NewStore().Get()
	if snap.Resources == nil {
		t.Fatal("Resources = nil, want empty slice")
	}
	if len(snap.Resources) != 0 {
		t.Errorf("Resources = %v, want empty", snap.Resources)
	}
}

func TestSetGetRoundtrip(t *testing.T) {
	st := NewStore()
	want := Snapshot{
		InSync:     true,
		Configured: true,
		Resources:  []Resource{{Kind: "Iso", Name: "debian-12", State: StateSynced}},
	}
	st.Set(want)

	got := st.Get()
	if !got.InSync || !got.Configured || len(got.Resources) != 1 || got.Resources[0].Name != "debian-12" {
		t.Errorf("Get = %+v, want %+v", got, want)
	}
}

func TestSetNormalizesNilResources(t *testing.T) {
	st := NewStore()
	st.Set(Snapshot{InSync: true})
	if st.Get().Resources == nil {
		t.Fatal("Resources = nil after Set, want empty slice")
	}
}

func TestSubscribeReceivesUpdates(t *testing.T) {
	st := NewStore()
	events, cancel := st.Subscribe()
	defer cancel()

	st.Set(Snapshot{InSync: true, Configured: true})

	select {
	case snap := <-events:
		if !snap.InSync || !snap.Configured {
			t.Errorf("received snapshot = %+v, want in sync and configured", snap)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	st := NewStore()
	events, cancel := st.Subscribe()
	cancel()

	st.Set(Snapshot{InSync: true})

	select {
	case snap := <-events:
		t.Errorf("unexpected event after cancel: %+v", snap)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSlowConsumerDropsOldest(t *testing.T) {
	st := NewStore()
	events, cancel := st.Subscribe()
	defer cancel()

	// The subscriber never reads; the broadcaster must never block, and the
	// buffer keeps only the most recent snapshots.
	const total = 20
	for i := range total {
		st.Set(Snapshot{UpdatedAt: time.Unix(int64(i), 0)})
	}

	if got := len(events); got != cap(events) {
		t.Errorf("buffered events = %d, want %d (capacity)", got, cap(events))
	}

	// The oldest retained snapshot is total-capacity: earlier ones were dropped.
	first := <-events
	if want := total - cap(events); !first.UpdatedAt.Equal(time.Unix(int64(want), 0)) {
		t.Errorf("first retained snapshot = %v, want %v", first.UpdatedAt, time.Unix(int64(want), 0))
	}
}
