package airflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPoolDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pools/mypool" {
			t.Fatalf("path %s", r.URL.Path)
		}
		u, p, ok := r.BasicAuth()
		if !ok || u != "u" || p != "p" {
			t.Fatalf("auth")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":            "mypool",
			"slots":           float64(32),
			"scheduled_slots": float64(2),
			"description":     "",
			"include_deferred": false,
		})
	}))
	defer srv.Close()

	t.Setenv("AIRFLOW_HOST", srv.URL)
	t.Setenv("AIRFLOW_USERNAME", "u")
	t.Setenv("AIRFLOW_PASSWORD", "p")
	t.Setenv("DRY_RUN", "false")

	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.GetPool("mypool")
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "mypool" || st.Slots != 32 {
		t.Fatalf("%+v", st)
	}
	if st.ScheduledSlots == nil || *st.ScheduledSlots != 2 {
		t.Fatalf("scheduled: %+v", st.ScheduledSlots)
	}
}

func TestPatchPoolSlotsDryRun(t *testing.T) {
	t.Setenv("AIRFLOW_HOST", "http://example.com")
	t.Setenv("AIRFLOW_USERNAME", "u")
	t.Setenv("AIRFLOW_PASSWORD", "p")
	t.Setenv("DRY_RUN", "true")
	c, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !c.DryRun() {
		t.Fatal("expected dry run")
	}
	cur := &PoolState{Name: "x", Slots: 5, Description: "d", IncludeDeferred: true}
	if err := c.PatchPoolSlots("x", cur, 10); err != nil {
		t.Fatal(err)
	}
}
