package store

import "testing"

// CountActiveJobs backs the desktop shell's quit guard: queued or running
// jobs die with the process, so the shell asks before closing over them.
func TestCountActiveJobs(t *testing.T) {
	st := newUsersStore(t)
	if n, err := st.CountActiveJobs(); err != nil || n != 0 {
		t.Fatalf("empty store: n=%d err=%v", n, err)
	}
	a, _ := st.AddJob("transfer", "a.bin", "s", "r", "d", "p", 1)
	b, _ := st.AddJob("transfer", "b.bin", "s", "r", "d", "p", 1)
	if n, _ := st.CountActiveJobs(); n != 2 {
		t.Fatalf("queued jobs count: n=%d", n)
	}
	if j, _ := st.ClaimNextQueuedJob(); j == nil {
		t.Fatal("claim should return a queued job")
	}
	if n, _ := st.CountActiveJobs(); n != 2 {
		t.Fatalf("running jobs still count: n=%d", n)
	}
	_ = st.FinishJob(a, "done", "")
	if n, _ := st.CountActiveJobs(); n != 1 {
		t.Fatalf("done excluded: n=%d", n)
	}
	_ = st.FinishJob(b, "failed", "boom")
	if n, _ := st.CountActiveJobs(); n != 0 {
		t.Fatalf("failed excluded: n=%d", n)
	}
}
