package relay

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Chunweiwu518/llm-gateway/backend/internal/billing"
	"github.com/Chunweiwu518/llm-gateway/backend/internal/store"
)

type fakeSettler struct {
	mu               sync.Mutex
	calls            int
	reserved, actual int64
	err              error
}

func (f *fakeSettler) Settle(_ context.Context, _ uint, reserved, actual int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.reserved, f.actual = reserved, actual
	if f.err != nil {
		return 0, f.err
	}
	return actual, nil
}

type fakeSink struct {
	mu   sync.Mutex
	rows []store.UsageLog
}

func (f *fakeSink) Record(e store.UsageLog) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, e)
}

func usageRows(g *gw) int64 {
	var n int64
	g.db.Model(&store.UsageLog{}).Where("user_id = ?", g.uid).Count(&n)
	return n
}

// With a Settler and a UsageSink wired, the handler settles through the Settler
// and hands the usage row to the sink instead of writing it to MySQL itself.
func TestChatUsesSettlerAndUsageSink(t *testing.T) {
	var hits int32
	g := newGateway(t, 10_000_000, fakeUpstream(t, 200, 0, &hits))
	st, sink := &fakeSettler{}, &fakeSink{}
	g.h.Settler, g.h.Usage = st, sink
	if w := g.chat(hello); w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
	want := billing.Cost(store.ModelPrice{InputPerM: 150_000, OutputPerM: 600_000}, 3, 2)
	if st.calls != 1 || st.actual != want || st.reserved < want {
		t.Fatalf("settler calls=%d reserved=%d actual=%d want actual %d", st.calls, st.reserved, st.actual, want)
	}
	if len(sink.rows) != 1 || sink.rows[0].Cost != want || sink.rows[0].StatusCode != 200 || sink.rows[0].UserID != g.uid {
		t.Fatalf("sink rows %+v", sink.rows)
	}
	if n := usageRows(g); n != 0 {
		t.Fatalf("usage row written to MySQL despite a sink: %d", n)
	}
}

// A failing billing service must not fail the caller's request.
func TestChatStillAnswersWhenSettlerFails(t *testing.T) {
	var hits int32
	g := newGateway(t, 10_000_000, fakeUpstream(t, 200, 0, &hits))
	g.h.Settler, g.h.Usage = &fakeSettler{err: errors.New("billing down")}, &fakeSink{}
	if w := g.chat(hello); w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
}
