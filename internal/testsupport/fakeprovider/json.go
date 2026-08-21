package fakeprovider

import (
	"encoding/json"
	"net/http"
	"testing"
)

func readJSONString(t *testing.T, r *http.Request, field string) string {
	t.Helper()

	var payload map[string]any

	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		t.Errorf("fakeprovider: decode %s %s: %v", r.Method, r.URL.Path, err)

		return ""
	}

	value, _ := payload[field].(string)

	return value
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")

	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	_, _ = w.Write(body)
}
